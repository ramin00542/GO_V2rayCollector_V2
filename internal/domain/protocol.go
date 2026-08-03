package domain

// Protocol identifies a format supported by the collector. New protocol parsers
// must also implement validation and canonical fingerprinting before enabling it.
type Protocol string

const (
	ProtocolVMess         Protocol = "vmess"
	ProtocolVLESS         Protocol = "vless"
	ProtocolTrojan        Protocol = "trojan"
	ProtocolShadowsocks   Protocol = "shadowsocks"
	ProtocolShadowsocksR  Protocol = "shadowsocksr"
	ProtocolHysteria      Protocol = "hysteria"
	ProtocolHysteria2     Protocol = "hysteria2"
	ProtocolTUIC          Protocol = "tuic"
	ProtocolWireGuard     Protocol = "wireguard"
	ProtocolWARP          Protocol = "warp"
	ProtocolSOCKS         Protocol = "socks"
	ProtocolSOCKS5        Protocol = "socks5"
	ProtocolHTTP          Protocol = "http"
	ProtocolHTTPS         Protocol = "https"
	ProtocolMTProto       Protocol = "mtproto"
	ProtocolTelegramSOCKS Protocol = "telegram_socks"
	ProtocolSSH           Protocol = "ssh"
	ProtocolOpenVPN       Protocol = "openvpn"
	ProtocolNaiveProxy    Protocol = "naiveproxy"
	ProtocolBrook         Protocol = "brook"
	ProtocolArgo          Protocol = "argo"
	ProtocolSlipnet       Protocol = "slipnet"
	ProtocolInvizible     Protocol = "invizible"
	ProtocolUnknown       Protocol = "unknown"
)

type ProtocolInfo struct {
	Protocol      Protocol
	FileName      string
	TelegramProxy bool
	MultiLine     bool
}

var protocolRegistry = map[Protocol]ProtocolInfo{
	ProtocolVMess:         {ProtocolVMess, "vmess.txt", false, false},
	ProtocolVLESS:         {ProtocolVLESS, "vless.txt", false, false},
	ProtocolTrojan:        {ProtocolTrojan, "trojan.txt", false, false},
	ProtocolShadowsocks:   {ProtocolShadowsocks, "shadowsocks.txt", false, false},
	ProtocolShadowsocksR:  {ProtocolShadowsocksR, "shadowsocksr.txt", false, false},
	ProtocolHysteria:      {ProtocolHysteria, "hysteria.txt", false, false},
	ProtocolHysteria2:     {ProtocolHysteria2, "hysteria2.txt", false, false},
	ProtocolTUIC:          {ProtocolTUIC, "tuic.txt", false, false},
	ProtocolWireGuard:     {ProtocolWireGuard, "wireguard.txt", false, true},
	ProtocolWARP:          {ProtocolWARP, "warp.txt", false, false},
	ProtocolSOCKS:         {ProtocolSOCKS, "socks.txt", false, false},
	ProtocolSOCKS5:        {ProtocolSOCKS5, "socks5.txt", false, false},
	ProtocolHTTP:          {ProtocolHTTP, "http.txt", false, false},
	ProtocolHTTPS:         {ProtocolHTTPS, "https.txt", false, false},
	ProtocolMTProto:       {ProtocolMTProto, "mtproto.txt", true, false},
	ProtocolTelegramSOCKS: {ProtocolTelegramSOCKS, "telegram-socks.txt", true, false},
	ProtocolSSH:           {ProtocolSSH, "ssh.txt", false, false},
	ProtocolOpenVPN:       {ProtocolOpenVPN, "openvpn.txt", false, true},
	ProtocolNaiveProxy:    {ProtocolNaiveProxy, "naiveproxy.txt", false, false},
	ProtocolBrook:         {ProtocolBrook, "brook.txt", false, false},
	ProtocolArgo:          {ProtocolArgo, "argo.txt", false, true},
	ProtocolSlipnet:       {ProtocolSlipnet, "slipnet.txt", false, false},
	ProtocolInvizible:     {ProtocolInvizible, "invizible.txt", false, false},
	ProtocolUnknown:       {ProtocolUnknown, "unknown.txt", false, false},
}

func ProtocolInfoFor(protocol Protocol) (ProtocolInfo, bool) {
	info, ok := protocolRegistry[protocol]
	return info, ok
}
