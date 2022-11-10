package shared

import (
	"github.com/ungtb10d/cli/v2/internal/config"
)

const (
	oauthToken = "oauth_token"
)

func AuthTokenWriteable(cfg config.Config, hostname string) (string, bool) {
	token, src := cfg.AuthToken(hostname)
	return src, (token == "" || src == oauthToken)
}
