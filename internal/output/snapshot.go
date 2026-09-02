package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

type SnapshotOptions struct {
	KeepUnknown     bool
	WritePerChannel bool
	WriteProtocols  bool
	WriteCombined   bool
}

func Publish(root string, entries []state.Entry, start, end time.Time, options SnapshotOptions) error {
	next := root + ".next"
	if err := os.RemoveAll(next); err != nil {
		return err
	}
	if err := writeSnapshot(next, entries, start, end, options); err != nil {
		os.RemoveAll(next)
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		os.RemoveAll(next)
		return err
	}
	return os.Rename(next, root)
}

func writeSnapshot(root string, entries []state.Entry, start, end time.Time, options SnapshotOptions) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("create snapshot root: %w", err)
	}
	files := make(map[string]map[string]bool)
	for _, entry := range entries {
		if entry.Protocol == domain.ProtocolUnknown && !options.KeepUnknown {
			continue
		}
		info, ok := domain.ProtocolInfoFor(entry.Protocol)
		if !ok {
			continue
		}
		for _, observation := range entry.Observations {
			if observation.LastSeenAt.Before(start) || !observation.LastSeenAt.Before(end) {
				continue
			}
			sourceDir := "subscription"
			if observation.Kind == domain.SourceTelegram {
				sourceDir = "telegram"
			}
			if options.WriteProtocols {
				directory := filepath.Join(root, sourceDir, "protocols")
				if info.TelegramProxy {
					directory = filepath.Join(root, sourceDir, "telegram-proxies")
				}
				add(files, filepath.Join(directory, info.FileName), entry.Value)
				if options.WritePerChannel && observation.Kind == domain.SourceTelegram && observation.Channel != "" {
					add(files, filepath.Join(root, "telegram", "channels", observation.Channel, info.FileName), entry.Value)
				}
			}
			// Combined all files are intended for VPN client subscriptions.
			// Generic HTTP/SOCKS and Telegram-native proxies remain available in
			// their dedicated protocol/proxy outputs, but never enter *_all.txt.
			if options.WriteCombined && isVPNProtocol(entry.Protocol) {
				add(files, filepath.Join(root, sourceDir+"_all.txt"), entry.Value)
			}
		}
	}
	for filename, values := range files {
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		items := make([]string, 0, len(values))
		for value := range values {
			items = append(items, value)
		}
		sort.Strings(items)
		if err := os.WriteFile(filename, []byte(strings.Join(items, "\n")+"\n"), 0644); err != nil {
			return fmt.Errorf("write output %s: %w", filename, err)
		}
	}
	return nil
}

func isVPNProtocol(protocol domain.Protocol) bool {
	// VPN protocols are those that provide full VPN functionality
	// and are suitable for inclusion in combined subscription files.
	// Telegram-native proxies (MTProto, TelegramSOCKS) and generic HTTP/SOCKS
	// proxies are NOT considered VPN protocols and should not enter *_all.txt files.
	switch protocol {
	case domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
		domain.ProtocolShadowsocks,
		domain.ProtocolShadowsocksR,
		domain.ProtocolHysteria,
		domain.ProtocolHysteria2,
		domain.ProtocolTUIC,
		domain.ProtocolWireGuard,
		domain.ProtocolWARP,
		domain.ProtocolNaiveProxy,
		domain.ProtocolBrook,
		domain.ProtocolArgo,
		domain.ProtocolSlipnet,
		domain.ProtocolInvizible:
		return true
	default:
		return false
	}
}

// isProxyProtocol returns true for protocols that are proxies rather than full VPNs
func isProxyProtocol(protocol domain.Protocol) bool {
	switch protocol {
	case domain.ProtocolHTTP,
		domain.ProtocolHTTPS,
		domain.ProtocolSOCKS,
		domain.ProtocolSOCKS5,
		domain.ProtocolMTProto,
		domain.ProtocolTelegramSOCKS,
		domain.ProtocolSSH:
		return true
	default:
		return false
	}
}

func add(files map[string]map[string]bool, filename, value string) {
	if files[filename] == nil {
		files[filename] = make(map[string]bool)
	}
	files[filename][value] = true
}

func DayBounds(day time.Time) (time.Time, time.Time) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}
