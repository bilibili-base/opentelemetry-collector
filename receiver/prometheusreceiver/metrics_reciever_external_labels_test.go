// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheusreceiver

import (
	"context"
	"testing"

	metricspb "github.com/census-instrumentation/opencensus-proto/gen-go/metrics/v1"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/translator/internaldata"
)

const targetExternalLabels = `
# HELP go_threads Number of OS threads created
# TYPE go_threads gauge
go_threads 19`

func TestExternalLabels(t *testing.T) {
	ctx := context.Background()
	targets := []*testData{
		{
			name: "target1",
			pages: []mockPrometheusResponse{
				{code: 200, data: targetExternalLabels},
			},
			validateFunc: verifyExternalLabels,
		},
	}

	mp, cfg, err := setupMockPrometheus(targets...)
	cfg.GlobalConfig.ExternalLabels = labels.FromStrings("key", "value")
	require.Nilf(t, err, "Failed to create Promtheus config: %v", err)
	defer mp.Close()

	cms := new(consumertest.MetricsSink)
	receiver := newPrometheusReceiver(logger, &Config{
		ReceiverSettings: config.NewReceiverSettings(config.NewID(typeStr)),
		PrometheusConfig: cfg}, cms)

	require.NoError(t, receiver.Start(ctx, componenttest.NewNopHost()), "Failed to invoke Start: %v", err)
	t.Cleanup(func() { require.NoError(t, receiver.Shutdown(ctx)) })

	mp.wg.Wait()
	metrics := cms.AllMetrics()

	results := make(map[string][]internaldata.MetricsData)
	for _, m := range metrics {
		ocmds := internaldata.MetricsToOC(m)
		for _, ocmd := range ocmds {
			result, ok := results[ocmd.Node.ServiceInfo.Name]
			if !ok {
				result = make([]internaldata.MetricsData, 0)
			}
			results[ocmd.Node.ServiceInfo.Name] = append(result, ocmd)
		}
	}
	for _, target := range targets {
		target.validateFunc(t, target, results[target.name])
	}
}

func verifyExternalLabels(t *testing.T, td *testData, mds []internaldata.MetricsData) {
	verifyNumScrapeResults(t, td, mds)

	wantG1 := &metricspb.Metric{
		MetricDescriptor: &metricspb.MetricDescriptor{
			Name:        "go_threads",
			Description: "Number of OS threads created",
			Type:        metricspb.MetricDescriptor_GAUGE_DOUBLE,
			LabelKeys:   []*metricspb.LabelKey{{Key: "key"}},
		},
		Timeseries: []*metricspb.TimeSeries{
			{
				Points: []*metricspb.Point{
					{Value: &metricspb.Point_DoubleValue{DoubleValue: 19.0}},
				},
				LabelValues: []*metricspb.LabelValue{
					{Value: "value", HasValue: true},
				},
			},
		},
	}
	gotG1 := mds[0].Metrics[0]
	ts1 := gotG1.Timeseries[0].Points[0].Timestamp
	wantG1.Timeseries[0].Points[0].Timestamp = ts1
	doCompare("scrape-externalLabels", t, wantG1, gotG1)
}