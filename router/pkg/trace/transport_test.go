package trace

import (
	"bytes"
	"context"
	"github.com/meistro2k/cosmo/router/pkg/otel"
	"github.com/meistro2k/cosmo/router/pkg/trace/tracetest"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

func TestTransport(t *testing.T) {

	t.Run("create a span for every request", func(t *testing.T) {
		content := []byte("Hello, world!")

		exporter := tracetest.NewInMemoryExporter(t)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write(content); err != nil {
				t.Fatal(err)
			}
		}))
		defer ts.Close()

		tsURL := ts.URL + "/test"
		r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tsURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		tr := NewTransport(http.DefaultTransport, []otelhttp.Option{
			otelhttp.WithSpanOptions(trace.WithAttributes(otel.WgComponentName.String("test"))),
			otelhttp.WithTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))),
		})

		c := http.Client{Transport: tr}
		res, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(body, content) {
			t.Fatalf("unexpected content: got %s, expected %s", body, content)
		}

		sn := exporter.GetSpans().Snapshots()
		assert.Len(t, sn, 1)
		assert.Equal(t, "HTTP GET", sn[0].Name())
		assert.Equal(t, trace.SpanKindClient, sn[0].SpanKind())
		assert.Equal(t, sdktrace.Status{Code: codes.Unset}, sn[0].Status())
		assert.Len(t, sn[0].Attributes(), 8)

		assert.Contains(t, sn[0].Attributes(), semconv.HTTPMethodKey.String("GET"))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPFlavorKey.String("1.1"))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPURL(tsURL))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPStatusCode(200))
		assert.Contains(t, sn[0].Attributes(), otel.WgComponentName.String("test"))
	})

	t.Run("set span status to error", func(t *testing.T) {
		content := []byte("Hello, world!")

		exporter := tracetest.NewInMemoryExporter(t)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write(content); err != nil {
				t.Fatal(err)
			}
		}))
		defer ts.Close()

		tsURL := ts.URL + "/test"
		r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tsURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		tr := NewTransport(http.DefaultTransport, []otelhttp.Option{
			otelhttp.WithSpanOptions(trace.WithAttributes(otel.WgComponentName.String("test"))),
			otelhttp.WithTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))),
		})

		c := http.Client{Transport: tr}
		res, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(body, content) {
			t.Fatalf("unexpected content: got %s, expected %s", body, content)
		}

		sn := exporter.GetSpans().Snapshots()
		assert.Len(t, sn, 1)
		assert.Equal(t, "HTTP GET", sn[0].Name())
		assert.Equal(t, trace.SpanKindClient, sn[0].SpanKind())
		assert.Equal(t, sdktrace.Status{Code: codes.Error}, sn[0].Status())
		assert.Len(t, sn[0].Attributes(), 8)

		assert.Contains(t, sn[0].Attributes(), semconv.HTTPMethodKey.String("GET"))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPFlavorKey.String("1.1"))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPURL(tsURL))
		assert.Contains(t, sn[0].Attributes(), semconv.HTTPStatusCode(http.StatusInternalServerError))
		assert.Contains(t, sn[0].Attributes(), otel.WgComponentName.String("test"))
	})
}
