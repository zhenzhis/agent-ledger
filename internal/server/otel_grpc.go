package server

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// OTLPTraceGRPCService accepts local OTLP/gRPC trace exports and projects GenAI
// spans into the same metadata-only ledger used by the HTTP OTLP receiver.
type OTLPTraceGRPCService struct {
	collectortracepb.UnimplementedTraceServiceServer
	db      *storage.DB
	options Options
}

func NewOTLPTraceGRPCService(db *storage.DB, options Options) *OTLPTraceGRPCService {
	return &OTLPTraceGRPCService{db: db, options: options}
}

func StartOTLPGRPCReceiver(db *storage.DB, cfg *config.Config) error {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	receiver := cfg.Integrations.OTLPReceiver
	if !receiver.Enabled || !receiver.GRPCEnabled {
		return fmt.Errorf("OTLP gRPC receiver is disabled")
	}
	if !loopbackBindAddress(receiver.GRPCBindAddress) {
		return fmt.Errorf("OTLP gRPC receiver refuses non-loopback bind address %q", receiver.GRPCBindAddress)
	}
	grpcPort := receiver.GRPCPort
	if grpcPort <= 0 {
		grpcPort = 4317
	}
	addr := fmt.Sprintf("%s:%d", firstNonEmpty(receiver.GRPCBindAddress, "127.0.0.1"), grpcPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	grpcServer := NewOTLPGRPCServer(db, Options{RBAC: cfg.RBAC, Integrations: cfg.Integrations})
	return grpcServer.Serve(lis)
}

func NewOTLPGRPCServer(db *storage.DB, options Options, opts ...grpc.ServerOption) *grpc.Server {
	receiver := options.Integrations.OTLPReceiver
	maxBody := receiver.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 4 << 20
	}
	opts = append([]grpc.ServerOption{grpc.MaxRecvMsgSize(int(maxBody))}, opts...)
	srv := grpc.NewServer(opts...)
	collectortracepb.RegisterTraceServiceServer(srv, NewOTLPTraceGRPCService(db, options))
	return srv
}

func (s *OTLPTraceGRPCService) Export(ctx context.Context, req *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	if !s.options.Integrations.OTLPReceiver.Enabled || !s.options.Integrations.OTLPReceiver.GRPCEnabled {
		return nil, status.Error(codes.NotFound, "OTLP gRPC receiver is disabled")
	}
	if s.options.RBAC.ReadOnly {
		return nil, status.Error(codes.PermissionDenied, "read-only mode: OTLP gRPC receiver writes are disabled")
	}
	raw, err := proto.Marshal(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "marshal OTLP request: %v", err)
	}
	spans, err := integrations.DecodeOTelProtoTraceSpans(raw)
	if err != nil {
		s.appendGRPCAudit("otlp.grpc.decode_error", "decode_error", map[string]string{"error": err.Error()})
		return nil, status.Errorf(codes.InvalidArgument, "decode OTLP request: %v", err)
	}
	maxSpans := s.options.Integrations.OTLPReceiver.MaxSpans
	if maxSpans <= 0 {
		maxSpans = 1000
	}
	if len(spans) > maxSpans {
		err := fmt.Errorf("OTLP span batch has %d spans and exceeds receiver limit %d", len(spans), maxSpans)
		s.appendGRPCAudit("otlp.grpc.backpressure", "span_limit_exceeded", map[string]string{"spans_seen": fmt.Sprint(len(spans)), "max_spans": fmt.Sprint(maxSpans)})
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	events, err := integrations.ConvertOTelGenAISpans(spans)
	if err != nil {
		s.appendGRPCAudit("otlp.grpc.convert_error", "convert_error", map[string]string{"error": err.Error(), "spans": fmt.Sprint(len(spans))})
		return nil, status.Errorf(codes.InvalidArgument, "convert OTLP spans: %v", err)
	}
	inserted := 0
	for _, event := range events {
		result, err := s.db.IngestCanonicalEvent(event)
		if err != nil {
			s.appendGRPCAudit("otlp.grpc.ingest_error", "ingest_error", map[string]string{"error": err.Error(), "spans": fmt.Sprint(len(spans)), "events": fmt.Sprint(len(events))})
			return nil, status.Errorf(codes.InvalidArgument, "ingest OTLP event: %v", err)
		}
		if result != nil && result.Status == "inserted" {
			inserted++
		}
	}
	s.appendGRPCAudit("otlp.grpc.ingest", fmt.Sprintf("%d", len(events)), map[string]string{"spans": fmt.Sprint(len(spans)), "events": fmt.Sprint(len(events)), "inserted": fmt.Sprint(inserted)})
	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

func (s *OTLPTraceGRPCService) appendGRPCAudit(action, target string, params map[string]string) {
	if s.options.RBAC.ReadOnly || s.db == nil {
		return
	}
	_ = s.db.AppendAuditLog("local", "operator", action, target, params)
}

func loopbackBindAddress(address string) bool {
	address = strings.Trim(strings.TrimSpace(address), "[]")
	if address == "" || strings.EqualFold(address, "localhost") {
		return true
	}
	if address == "0.0.0.0" || address == "::" || address == "*" {
		return false
	}
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
