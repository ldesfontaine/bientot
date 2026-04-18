package client

import (
	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/modules"
)

// ToProto converts a modules.Data (agent-side Go struct) into the protobuf
// ModuleData used on the wire. Keeps the module contract decoupled from proto.
func ToProto(data *modules.Data) *bientotv1.ModuleData {
	if data == nil {
		return nil
	}

	md := &bientotv1.ModuleData{
		Module:      data.Module,
		TimestampNs: data.Timestamp.UnixNano(),
		Metadata:    data.Metadata,
	}

	for _, m := range data.Metrics {
		md.Metrics = append(md.Metrics, &bientotv1.Metric{
			Name:   m.Name,
			Value:  m.Value,
			Labels: m.Labels,
		})
	}

	for _, s := range data.Software {
		md.Software = append(md.Software, &bientotv1.SoftwareItem{
			Name:    s.Name,
			Version: s.Version,
			Source:  s.Source,
		})
	}

	for _, ev := range data.RawEvents {
		fields := make(map[string]string, len(ev.Fields))
		for k, v := range ev.Fields {
			if s, ok := v.(string); ok {
				fields[k] = s
			}
		}
		md.RawEvents = append(md.RawEvents, &bientotv1.RawEvent{
			Source:      ev.Source,
			TimestampNs: ev.Timestamp.UnixNano(),
			Fields:      fields,
		})
	}

	return md
}
