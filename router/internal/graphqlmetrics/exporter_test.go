package graphqlmetrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	graphqlmetricsv1 "github.com/meistro2k/cosmo/router/gen/proto/wg/cosmo/graphqlmetrics/v1"
	"github.com/meistro2k/cosmo/router/gen/proto/wg/cosmo/graphqlmetrics/v1/graphqlmetricsv1connect"
	"go.uber.org/zap"
)

type MyClient struct {
	t                     *testing.T
	publishedBatches      [][]*graphqlmetricsv1.SchemaUsageInfo
	publishedAggregations [][]*graphqlmetricsv1.SchemaUsageInfoAggregation
	mu                    sync.Mutex
}

func (m *MyClient) PublishAggregatedGraphQLMetrics(ctx context.Context, c *connect.Request[graphqlmetricsv1.PublishAggregatedGraphQLRequestMetricsRequest]) (*connect.Response[graphqlmetricsv1.PublishAggregatedGraphQLRequestMetricsResponse], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	require.Equal(m.t, "Bearer secret", c.Header().Get("Authorization"))
	m.publishedAggregations = append(m.publishedAggregations, c.Msg.Aggregation)
	return nil, nil
}

func (m *MyClient) PublishGraphQLMetrics(ctx context.Context, c *connect.Request[graphqlmetricsv1.PublishGraphQLRequestMetricsRequest]) (*connect.Response[graphqlmetricsv1.PublishOperationCoverageReportResponse], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	require.Equal(m.t, "Bearer secret", c.Header().Get("Authorization"))
	m.publishedBatches = append(m.publishedBatches, c.Msg.GetSchemaUsage())
	return nil, nil
}

var _ graphqlmetricsv1connect.GraphQLMetricsServiceClient = (*MyClient)(nil)

func TestExportAggregationSameSchemaUsages(t *testing.T) {
	c := &MyClient{
		t: t,
	}

	queueSize := 200
	totalItems := 100
	batchSize := 100

	e, err := NewExporter(
		zap.NewNop(),
		c,
		"secret",
		&ExporterSettings{
			BatchSize: batchSize,
			QueueSize: queueSize,
			Interval:  500 * time.Millisecond,
			RetryOptions: RetryOptions{
				Enabled:     false,
				MaxDuration: 300 * time.Millisecond,
				Interval:    100 * time.Millisecond,
				MaxRetry:    3,
			},
			ExportTimeout: 100 * time.Millisecond,
		},
	)

	require.Nil(t, err)

	for i := 0; i < totalItems; i++ {

		hash := fmt.Sprintf("hash-%d", i%2)

		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
					Count:       1,
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: hash,
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			RequestInfo: &graphqlmetricsv1.RequestInfo{
				Error:      false,
				StatusCode: http.StatusOK,
			},
			Attributes: map[string]string{
				"client_name":    "wundergraph",
				"client_version": "1.0.0",
			},
		}

		require.True(t, e.RecordUsage(usage, false))
	}

	require.Nil(t, e.Shutdown(context.Background()))

	require.False(t, e.RecordUsage(nil, false))

	c.mu.Lock()
	defer c.mu.Unlock()
	require.Equal(t, 1, len(c.publishedAggregations))
	require.Equal(t, 2, len(c.publishedAggregations[0]))
	require.Equal(t, 50, int(c.publishedAggregations[0][0].RequestCount))
	require.Equal(t, 50, int(c.publishedAggregations[0][1].RequestCount))
}

func TestExportBatchesWithUniqueSchemaUsages(t *testing.T) {
	c := &MyClient{
		t: t,
	}

	queueSize := 200
	totalItems := 100
	batchSize := 5

	e, err := NewExporter(
		zap.NewNop(),
		c,
		"secret",
		&ExporterSettings{
			BatchSize: batchSize,
			QueueSize: queueSize,
			Interval:  time.Second * 5,
			RetryOptions: RetryOptions{
				Enabled:     false,
				MaxDuration: 300 * time.Millisecond,
				Interval:    100 * time.Millisecond,
				MaxRetry:    3,
			},
			ExportTimeout: 100 * time.Millisecond,
		},
	)

	require.Nil(t, err)

	for i := 0; i < totalItems; i++ {
		i := i
		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "id"},
					TypeNames:   []string{"User", "ID"},
					SubgraphIDs: []string{"1", "2"},
				},
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: fmt.Sprintf("hash-%d", i),
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			Attributes: map[string]string{},
		}

		e.RecordUsage(usage, false)
	}

	require.Nil(t, e.Shutdown(context.Background()))
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Equal(t, totalItems/batchSize, len(c.publishedAggregations))
}

func TestForceFlushSync(t *testing.T) {
	c := &MyClient{
		t:                t,
		publishedBatches: make([][]*graphqlmetricsv1.SchemaUsageInfo, 0),
	}

	queueSize := 100
	totalItems := 10
	batchSize := 5

	e, err := NewExporter(
		zap.NewNop(),
		c,
		"secret",
		&ExporterSettings{
			BatchSize: batchSize,
			QueueSize: queueSize,
			// Intentionally set to a high value to make sure that the exporter is forced to flush immediately
			Interval:      5000 * time.Millisecond,
			ExportTimeout: 5000 * time.Millisecond,
			RetryOptions: RetryOptions{
				Enabled:     false,
				MaxDuration: 300 * time.Millisecond,
				Interval:    100 * time.Millisecond,
				MaxRetry:    3,
			},
		},
	)

	require.Nil(t, err)

	for i := 0; i < totalItems; i++ {
		i := i
		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "id"},
					TypeNames:   []string{"User", "ID"},
					SubgraphIDs: []string{"1", "2"},
					Count:       2,
				},
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
					Count:       1,
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: fmt.Sprintf("hash-%d", i),
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			Attributes: map[string]string{},
		}

		e.RecordUsage(usage, true)
	}

	c.mu.Lock()
	require.Equal(t, 10, len(c.publishedBatches))
	require.Equal(t, 1, len(c.publishedBatches[0]))

	// Make sure that the exporter is still working after a forced flush

	// Reset the published batches
	c.publishedBatches = c.publishedBatches[:0]
	c.mu.Unlock()

	for i := 0; i < totalItems; i++ {
		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "id"},
					TypeNames:   []string{"User", "ID"},
					SubgraphIDs: []string{"1", "2"},
					Count:       2,
				},
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
					Count:       1,
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: fmt.Sprintf("hash-%d", i),
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			Attributes: map[string]string{},
		}

		e.RecordUsage(usage, true)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	require.Equal(t, 10, len(c.publishedBatches))
	require.Equal(t, 1, len(c.publishedBatches[0]))
}

func TestExportBatchInterval(t *testing.T) {
	c := &MyClient{
		t: t,
	}

	queueSize := 200
	totalItems := 5
	batchSize := 10

	e, err := NewExporter(
		zap.NewNop(),
		c,
		"secret",
		&ExporterSettings{
			BatchSize: batchSize,
			QueueSize: queueSize,
			Interval:  100 * time.Millisecond,
			RetryOptions: RetryOptions{
				Enabled:     false,
				MaxDuration: 300 * time.Millisecond,
				Interval:    100 * time.Millisecond,
				MaxRetry:    3,
			},
			ExportTimeout: 100 * time.Millisecond,
		},
	)

	require.Nil(t, err)

	for i := 0; i < totalItems; i++ {
		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "id"},
					TypeNames:   []string{"User", "ID"},
					SubgraphIDs: []string{"1", "2"},
					Count:       2,
				},
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
					Count:       1,
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: fmt.Sprintf("hash-%d", i),
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			Attributes: map[string]string{},
		}

		e.RecordUsage(usage, false)
	}

	time.Sleep(200 * time.Millisecond)

	defer require.Nil(t, e.Shutdown(context.Background()))

	require.Equal(t, 1, len(c.publishedAggregations))
	require.Equal(t, 5, len(c.publishedAggregations[0]))
}

func TestExportFullQueue(t *testing.T) {
	c := &MyClient{
		t: t,
	}

	// limits are too low, so queue will be blocked
	queueSize := 2
	totalItems := 100
	batchSize := 1

	e, err := NewExporter(
		zap.NewNop(),
		c,
		"secret",
		&ExporterSettings{
			BatchSize: batchSize,
			QueueSize: queueSize,
			Interval:  500 * time.Millisecond,
			RetryOptions: RetryOptions{
				Enabled:     false,
				MaxDuration: 300 * time.Millisecond,
				Interval:    100 * time.Millisecond,
				MaxRetry:    3,
			},
			ExportTimeout: 100 * time.Millisecond,
		},
	)

	require.Nil(t, err)

	var dispatched int

	for i := 0; i < totalItems; i++ {

		usage := &graphqlmetricsv1.SchemaUsageInfo{
			TypeFieldMetrics: []*graphqlmetricsv1.TypeFieldUsageInfo{
				{
					Path:        []string{"user", "name"},
					TypeNames:   []string{"User", "String"},
					SubgraphIDs: []string{"1", "2"},
					Count:       1,
				},
			},
			OperationInfo: &graphqlmetricsv1.OperationInfo{
				Type: graphqlmetricsv1.OperationType_QUERY,
				Hash: "hash",
				Name: "user",
			},
			ClientInfo: &graphqlmetricsv1.ClientInfo{
				Name:    "wundergraph",
				Version: "1.0.0",
			},
			SchemaInfo: &graphqlmetricsv1.SchemaInfo{
				Version: "1",
			},
			Attributes: map[string]string{
				"client_name":    "wundergraph",
				"client_version": "1.0.0",
			},
		}

		if e.RecordUsage(usage, false) {
			dispatched++
		}
	}

	require.Nil(t, e.Shutdown(context.Background()))

	require.Lessf(t, dispatched, 100, "expect less than 100 batches, because queue is full")
}
