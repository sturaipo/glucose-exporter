package librelink

import (
	"encoding/json"
	"time"
)

const DateFormat = "1/2/2006 3:04:05 PM"

type LibreLinkResp struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
	Ticket AuthTicket      `json:"ticket,omitempty"`
	Error  *LibreLinkError `json:"error,omitempty"`
}

type LibreLinkError struct {
	Message string `json:"message"`
}

type User struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Country   string `json:"country"`

	// other fields omitted for brevity
	// https://libreview-unofficial.stoplight.io/docs/libreview-unofficial/n7k5paczc1woz-user-data
}

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthTicket struct {
	Token    string        `json:"token"`
	Expires  time.Time     `json:"expires"`
	Duration time.Duration `json:"duration"`
}

func (t *AuthTicket) UnmarshalJSON(data []byte) error {
	type Alias AuthTicket
	aux := &struct {
		Expires  int64 `json:"expires"`
		Duration int64 `json:"duration"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.Expires = time.Unix(aux.Expires, 0)
	t.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}

type AuthResponse struct {
	User       User       `json:"user"`
	AuthTicket AuthTicket `json:"authTicket"`
}

func parseDate(s string) (time.Time, error) {
	return time.Parse(DateFormat, s)
}

type RedirecResponse struct {
	Redirect bool   `json:"redirect"`
	Region   string `json:"region"`
}

func getPayload[T any](resp LibreLinkResp, out *T) error {
	return json.Unmarshal(resp.Data, &out)
}

type Connection struct {
	Id        string `json:"id"`
	PatientId string `json:"patientId"`

	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`

	DateOfBirth   string `json:"dateOfBirth"`
	EmailVerified bool   `json:"emailVerified"`
	SignUpDate    string `json:"signUpDate"`

	GlucoseMeasurement *GlucoseMeasurement `json:"glucoseMeasurement,omitempty"`
	GlucoseItem        *GlucoseMeasurement `json:"glucoseItem,omitempty"`
}

type GlucoseUnits int

const (
	GlucoseUnitsMgPerDl  GlucoseUnits = 1
	GlucoseUnitsMmolPerL GlucoseUnits = 0
)

type GlucoseArrow int

const (
	GlucoseArrowNone GlucoseArrow = iota
	GlucoseArrowDown
	GlucoseArrowDownRight
	GlucoseArrowRight
	GlucoseArrowUpRight
	GlucoseArrowUp
)

type GlucoseMeasurement struct {
	Timestamp        time.Time    `json:"FactoryTimestamp"`
	Type             int          `json:"type"`
	ValueInMgPerDl   int          `json:"ValueInMgPerDl"`
	MeasurementColor int          `json:"MeasurementColor"`
	GlucoseUnits     GlucoseUnits `json:"GlucoseUnits"`
	Value            float64      `json:"Value"`
	IsHigh           bool         `json:"isHigh"`
	IsLow            bool         `json:"isLow"`
	TrendArrow       int          `json:"TrendArrow"`
	TrendMessage     *string      `json:"TrendMessage"`
}

func (g *GlucoseMeasurement) UnmarshalJSON(data []byte) error {
	type Alias GlucoseMeasurement
	aux := &struct {
		Timestamp string `json:"FactoryTimestamp"`
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t, err := parseDate(aux.Timestamp)
	if err != nil {
		return err
	}
	g.Timestamp = t
	return nil
}

type GraphData struct {
	Connection Connection           `json:"connection"`
	GraphData  []GlucoseMeasurement `json:"graphData"`
}
