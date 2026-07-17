package proxy

// ProxyConfig holds the per-charger upstream relay configuration.
// Stored in the charger_proxy_config table.
type ProxyConfig struct {
	ChargerID                string
	ProxyEnabled             bool
	UpstreamURL              string
	UpstreamUser             string
	UpstreamPasswordEnc      []byte
	ProxyPolicyJSON          string
	UpstreamThrottleMVPerMin int
}
