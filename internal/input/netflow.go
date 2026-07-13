package input

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/tehmaze/netflow"
	"github.com/tehmaze/netflow/ipfix"
	"github.com/tehmaze/netflow/netflow5"
	"github.com/tehmaze/netflow/netflow9"
	"github.com/tehmaze/netflow/session"
)

// nfCanon maps well-known NetFlow/IPFIX element names to canonical fields.
var nfCanon = map[string]string{
	"sourceIPv4Address":        "src_ip",
	"destinationIPv4Address":   "dst_ip",
	"sourceIPv6Address":        "src_ip",
	"destinationIPv6Address":   "dst_ip",
	"sourceTransportPort":      "src_port",
	"destinationTransportPort": "dst_port",
	"protocolIdentifier":       "proto",
	"octetDeltaCount":          "bytes",
	"packetDeltaCount":         "packets",
}

// NetFlow receives NetFlow v5/v9 and IPFIX over UDP, decoding flows at ingest
// (templates arrive in-band, per exporter) and journaling each flow as JSON.
type NetFlow struct {
	cfg Config
	pub Publisher
	log *slog.Logger
	srv *netServer

	mu       sync.Mutex
	decoders map[string]*netflow.Decoder // per source exporter
}

// NewNetFlow builds a NetFlow/IPFIX input.
func NewNetFlow(cfg Config, pub Publisher, log *slog.Logger) *NetFlow {
	return &NetFlow{cfg: cfg, pub: pub, log: log, decoders: map[string]*netflow.Decoder{}}
}

func (n *NetFlow) ID() string   { return n.cfg.ID }
func (n *NetFlow) Type() string { return "netflow" }

func (n *NetFlow) Start() error {
	n.srv = &netServer{
		proto:    UDP,
		bind:     n.cfg.Bind,
		log:      n.log,
		onPacket: n.onPacket,
	}
	return n.srv.start()
}

func (n *NetFlow) Stop(ctx context.Context) error {
	if n.srv == nil {
		return nil
	}
	return n.srv.stop(ctx)
}

// decoderFor returns a per-exporter decoder so template state does not collide.
func (n *NetFlow) decoderFor(source string) *netflow.Decoder {
	n.mu.Lock()
	defer n.mu.Unlock()
	d, ok := n.decoders[source]
	if !ok {
		d = netflow.NewDecoder(session.New())
		n.decoders[source] = d
	}
	return d
}

func (n *NetFlow) onPacket(payload []byte, source string) {
	msg, err := n.decoderFor(source).Read(bytes.NewReader(payload))
	if err != nil {
		n.log.Debug("netflow decode error", "source", source, "error", err)
		return
	}
	switch p := msg.(type) {
	case *netflow5.Packet:
		for _, rec := range p.Records {
			n.emit(v5Fields(rec), source, "netflow")
		}
	case *netflow9.Packet:
		for _, set := range p.DataFlowSets {
			for _, rec := range set.Records {
				n.emit(translatedFields9(rec.Fields), source, "netflow")
			}
		}
	case *ipfix.Message:
		for _, set := range p.DataSets {
			for _, rec := range set.Records {
				n.emit(translatedFieldsIPFIX(rec.Fields), source, "ipfix")
			}
		}
	}
}

func (n *NetFlow) emit(fields map[string]any, source, typ string) {
	if len(fields) == 0 {
		return
	}
	fields["message"] = flowSummary(fields)
	payload, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_ = n.pub.Publish(RawMessage{
		InputID:    n.cfg.ID,
		InputType:  typ,
		Source:     source,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

func v5Fields(r *netflow5.FlowRecord) map[string]any {
	return map[string]any{
		"src_ip":    r.SrcAddr.String(),
		"dst_ip":    r.DstAddr.String(),
		"src_port":  r.SrcPort,
		"dst_port":  r.DstPort,
		"proto":     r.Protocol,
		"packets":   r.Packets,
		"bytes":     r.Bytes,
		"tcp_flags": r.TCPFlags,
	}
}

func translatedFields9(fields netflow9.Fields) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if f.Translated == nil || f.Translated.Name == "" {
			continue
		}
		addField(out, f.Translated.Name, f.Translated.Value)
	}
	return out
}

func translatedFieldsIPFIX(fields ipfix.Fields) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if f.Translated == nil || f.Translated.Name == "" {
			continue
		}
		addField(out, f.Translated.Name, f.Translated.Value)
	}
	return out
}

func addField(out map[string]any, name string, value any) {
	out[name] = value
	if canon, ok := nfCanon[name]; ok {
		out[canon] = value
	}
}

func flowSummary(f map[string]any) string {
	return fmt.Sprintf("%v:%v -> %v:%v proto=%v bytes=%v packets=%v",
		f["src_ip"], f["src_port"], f["dst_ip"], f["dst_port"], f["proto"], f["bytes"], f["packets"])
}
