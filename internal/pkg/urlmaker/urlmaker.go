package urlmaker

import (
	"fmt"
	"strings"
)

func GetSiteURL(protocol string, siteID string, domain string) string {
	return fmt.Sprintf("%s://%s", protocol, GetSiteFQDN(siteID, domain))
}

func GetSiteFQDN(siteID string, domain string) string {
	if domain == "" {
		return strings.ToLower(siteID)
	}
	return fmt.Sprintf("%s.%s", strings.ToLower(siteID), domain)
}
