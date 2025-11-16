package collector

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sturaipo/glucose-exporter/api/librelink"
)

type GlucoseCollector struct {
	client *librelink.LibreLinkClient

	glucoseLevelDesc *prometheus.Desc
	trendDesc        *prometheus.Desc
	historicDataDesc *prometheus.Desc
}

func NewGlucoseCollector(client *librelink.LibreLinkClient) *GlucoseCollector {
	return &GlucoseCollector{
		client: client,
		glucoseLevelDesc: prometheus.NewDesc(
			prometheus.BuildFQName("glucose", "librelink", "level_mmoll"),
			"Current glucose level in mmmol/L",
			[]string{"patient_id", "patient_name"},
			nil,
		),
		trendDesc: prometheus.NewDesc(
			prometheus.BuildFQName("glucose", "librelink", "trend"),
			"Current glucose trend",
			[]string{"patient_id", "patient_name"},
			nil,
		),
		historicDataDesc: prometheus.NewDesc(
			prometheus.BuildFQName("glucose", "librelink", "historic_level"),
			"Historic glucose data",
			[]string{"patient_id", "patient_name"},
			nil,
		),
	}
}

func (gc GlucoseCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(gc, ch)
}

func (gc GlucoseCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()

	if !gc.client.IsAuthenticated() {
		if err := gc.client.Authenticate(ctx); err != nil {
			return
		}
	}

	conn, err := gc.client.GetConnections(ctx)
	if err != nil {
		return
	}

	for _, c := range conn {
		gc.collectGlucose(ctx, ch, c)
	}
}

func (gc GlucoseCollector) collectGlucose(ctx context.Context, ch chan<- prometheus.Metric, connection librelink.Connection) {

	data, err := gc.client.GetGraphData(ctx, connection.PatientId)
	if err != nil {
		return
	}

	reading := data.Connection.GlucoseMeasurement
	if reading == nil {
		return
	}

	patient_name := fmt.Sprintf("%s %s", connection.FirstName, connection.LastName)

	ch <- prometheus.NewMetricWithTimestamp(
		reading.Timestamp,
		prometheus.MustNewConstMetric(
			gc.glucoseLevelDesc,
			prometheus.GaugeValue,
			reading.Value,
			connection.PatientId,
			patient_name,
		),
	)

	ch <- prometheus.NewMetricWithTimestamp(
		reading.Timestamp,
		prometheus.MustNewConstMetric(
			gc.trendDesc,
			prometheus.GaugeValue,
			float64(reading.TrendArrow),
			connection.PatientId,
			patient_name,
		),
	)

	for _, historic := range data.GraphData {
		ch <- prometheus.NewMetricWithTimestamp(
			historic.Timestamp,
			prometheus.MustNewConstMetric(
				gc.historicDataDesc,
				prometheus.GaugeValue,
				historic.Value,
				connection.PatientId,
				patient_name,
			),
		)
	}
}
