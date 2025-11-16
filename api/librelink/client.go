package librelink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

func getUrl(region string) string {
	if region == "" {
		return "https://api.libreview.io"
	}
	return fmt.Sprintf("https://api-%s.libreview.io", region)
}

type librelinkCreds struct {
	ticket AuthTicket
	id     string
}

func NewLibreLinkCreds(userId string, ticket AuthTicket) *librelinkCreds {
	hashed := sha256.Sum256([]byte(userId))
	return &librelinkCreds{
		ticket: ticket,
		id:     hex.EncodeToString(hashed[:]),
	}
}

type LibreLinkClient struct {
	baseUrl    *url.URL
	httpClient *http.Client
	logger     *zap.Logger

	user     string
	password string

	creds *librelinkCreds
}

func WithCredentials(userId string, token string) func(*LibreLinkClient) {
	return WithExpiringCredentials(userId, token, time.Time{})
}

func WithExpiringCredentials(userId string, token string, expiry time.Time) func(*LibreLinkClient) {
	return func(c *LibreLinkClient) {
		c.creds = NewLibreLinkCreds(
			userId,
			AuthTicket{
				Token:   token,
				Expires: expiry,
			},
		)
	}
}

func WithLogger(logger *zap.Logger) func(*LibreLinkClient) {
	return func(c *LibreLinkClient) {
		c.logger = logger
	}
}

func NewLibreLinkClient(user string, password string, options ...func(*LibreLinkClient)) *LibreLinkClient {
	baseUrl, _ := url.Parse("https://api.libreview.io")
	client := &LibreLinkClient{
		baseUrl:    baseUrl,
		httpClient: &http.Client{},

		user:     user,
		password: password,

		logger: zap.NewNop(),
	}

	for _, option := range options {
		option(client)
	}

	return client
}

func (c *LibreLinkClient) handleRedirect(resp LibreLinkResp) (bool, error) {
	redirect := RedirecResponse{}
	if err := getPayload(resp, &redirect); err != nil {
		return false, nil
	}
	if !redirect.Redirect {
		return false, nil
	}

	if redirect.Region == "" {
		return false, fmt.Errorf("redirect requested but no region provided")
	}

	baseUrl, err := url.Parse(getUrl(redirect.Region))
	if err != nil {
		return false, fmt.Errorf("failed to parse redirect URL: %w", err)
	}
	c.baseUrl = baseUrl
	return true, nil
}

func (c LibreLinkClient) prepareRequest(ctx context.Context, method string, endpoint string, body io.Reader) (*http.Request, error) {
	rel := c.baseUrl.JoinPath(endpoint)

	logger := c.logger.With(
		zap.String("method", method),
		zap.String("url", rel.String()),
	)

	logger.Debug("Preparing request")

	req, err := http.NewRequestWithContext(ctx, method, rel.String(), body)

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("product", "llu.android")
	req.Header.Set("version", "4.16.0")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("connection", "Keep-Alive")

	if c.creds != nil {
		logger.Info("Using existing credentials")
		req.Header.Set("account-id", c.creds.id)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.creds.ticket.Token))
	} else {
		logger.Info("No credentials available, proceeding without authentication")
	}

	return req, nil
}

func (c *LibreLinkClient) doRequest(ctx context.Context, method string, endpoint string, body io.Reader) (LibreLinkResp, error) {
	logger := c.logger.With(
		zap.String("method", method),
		zap.String("url", endpoint),
	)

	logger.Debug("Preparing to do request")
	req, err := c.prepareRequest(ctx, method, endpoint, body)

	if err != nil {
		return LibreLinkResp{}, fmt.Errorf("failed to prepare request: %w", err)
	}

	logger.Debug("Executing request")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return LibreLinkResp{}, fmt.Errorf("request failed: %w", err)
	}

	defer resp.Body.Close()

	logger.Debug("Request completed", zap.Int("status", resp.StatusCode))
	if resp.StatusCode != http.StatusOK {
		return LibreLinkResp{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var libreResp LibreLinkResp
	raw, _ := io.ReadAll(resp.Body)
	logger.Debug("Response body", zap.ByteString("body", raw))

	if err := json.Unmarshal(raw, &libreResp); err != nil {
		return LibreLinkResp{}, fmt.Errorf("failed to decode response body: %w", err)
	}

	logger.Debug("Response parsed", zap.Int("libre_status", libreResp.Status))
	if libreResp.Status != 0 {
		return LibreLinkResp{}, fmt.Errorf("API error %d: %s", libreResp.Status, libreResp.Error.Message)
	}

	redirected, err := c.handleRedirect(libreResp)
	if err != nil {
		return LibreLinkResp{}, fmt.Errorf("failed to handle redirect: %w", err)
	}

	if redirected {
		logger.Info("Redirected to new region, retrying request", zap.String("new_base_url", c.baseUrl.String()))
		// retry the request with the new base URL
		return c.doRequest(ctx, method, endpoint, body)
	}

	return libreResp, nil
}

func (c *LibreLinkClient) Authenticate(ctx context.Context) error {
	endpoint := "llu/auth/login"

	AuthRequest := AuthRequest{
		Email:    c.user,
		Password: c.password,
	}

	reqBody, err := json.Marshal(AuthRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	var authResp AuthResponse
	if err := json.Unmarshal(resp.Data, &authResp); err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	c.creds = NewLibreLinkCreds(authResp.User.ID, authResp.AuthTicket)
	return nil
}

func (c *LibreLinkClient) GetConnections(ctx context.Context) ([]Connection, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "llu/connections", nil)

	if err != nil {
		return nil, err
	}

	var connections []Connection
	if err := getPayload(resp, &connections); err != nil {
		return nil, fmt.Errorf("failed to decode connections: %w", err)
	}

	return connections, nil
}

func (c *LibreLinkClient) getGraphData(ctx context.Context, connectionId string) (GraphData, error) {
	endpoint := fmt.Sprintf("llu/connections/%s/graph", connectionId)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)

	if err != nil {
		return GraphData{}, err
	}

	var graphData GraphData
	if err := getPayload(resp, &graphData); err != nil {
		return GraphData{}, fmt.Errorf("failed to decode graph data: %w", err)
	}

	return graphData, nil
}

func (c *LibreLinkClient) GetLatestReading(ctx context.Context, connectionId string) (GlucoseMeasurement, error) {
	graphData, err := c.getGraphData(ctx, connectionId)
	if err != nil {
		return GlucoseMeasurement{}, err
	}

	return *graphData.Connection.GlucoseMeasurement, nil
}

func (c *LibreLinkClient) IsAuthenticated() bool {
	return c.creds != nil
}
