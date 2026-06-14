package server

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestOTLPGRPCReceiverIngestsProtobufSpans(t *testing.T) {
	db := testServerDB(t)
	client := newOTLPGRPCTestClient(t, db, Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, GRPCEnabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 10},
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Export(ctx, otlpProtoRequest(t, "span-1")); err != nil {
		t.Fatalf("Export: %v", err)
	}

	from := time.Unix(0, 1780836000000000000).UTC().Add(-time.Second)
	to := from.Add(2 * time.Second)
	stats, err := db.GetDashboardStatsFiltered(from, to, "otlp-protobuf-test", "gpt-5.5", "quant")
	if err != nil {
		t.Fatalf("GetDashboardStatsFiltered: %v", err)
	}
	if stats.TotalCalls != 1 || stats.TotalTokens != 15 || stats.TotalSessions != 1 {
		t.Fatalf("unexpected projected usage stats: %+v", stats)
	}
	assertAuditEvent(t, db, "otlp.grpc.ingest", "2")
}

func TestOTLPGRPCReceiverRejectsReadOnly(t *testing.T) {
	db := testServerDB(t)
	client := newOTLPGRPCTestClient(t, db, Options{
		RBAC: config.RBACConfig{ReadOnly: true},
		Integrations: config.IntegrationsConfig{
			OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, GRPCEnabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 10},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Export(ctx, otlpProtoRequest(t, "span-1"))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got code=%s err=%v", status.Code(err), err)
	}
	rows, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("read-only gRPC receiver should not write audit rows: %+v", rows)
	}
}

func TestOTLPGRPCReceiverRejectsSpanLimit(t *testing.T) {
	db := testServerDB(t)
	client := newOTLPGRPCTestClient(t, db, Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, GRPCEnabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 1},
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Export(ctx, otlpProtoRequest(t, "span-1", "span-2"))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted, got code=%s err=%v", status.Code(err), err)
	}
	assertAuditEvent(t, db, "otlp.grpc.backpressure", "span_limit_exceeded")
}

func TestStartOTLPGRPCReceiverRejectsNonLoopback(t *testing.T) {
	db := testServerDB(t)
	cfg := config.DefaultConfig()
	cfg.Integrations.OTLPReceiver.Enabled = true
	cfg.Integrations.OTLPReceiver.GRPCEnabled = true
	cfg.Integrations.OTLPReceiver.GRPCBindAddress = "0.0.0.0"

	err := StartOTLPGRPCReceiver(db, cfg)
	if err == nil || !strings.Contains(err.Error(), "refuses non-loopback") {
		t.Fatalf("expected non-loopback bind rejection, got %v", err)
	}
}

func newOTLPGRPCTestClient(t *testing.T, db *storage.DB, options Options) collectortracepb.TraceServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	grpcServer := NewOTLPGRPCServer(db, options)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = lis.Close()
	})
	conn, err := grpc.NewClient("passthrough:///agent-ledger-otlp-grpc",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return collectortracepb.NewTraceServiceClient(conn)
}

func assertAuditEvent(t *testing.T, db *storage.DB, action, target string) {
	t.Helper()
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	for _, event := range events {
		if event.Action == action && event.Target == target {
			return
		}
	}
	t.Fatalf("missing audit event %s/%s: %+v", action, target, events)
}
