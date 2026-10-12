// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package citation

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
)

// newsHostsJSON lists the hosts of well-known news publications: a page there is cited as a
// news article. The editor reads the same file (web_src/js/features/citations), so the type
// it guesses from a pasted URL matches the one the lookup returns.
//
//go:embed news_hosts.json
var newsHostsJSON []byte

var newsHosts = sync.OnceValue(func() map[string]bool {
	var hosts []string
	if err := json.Unmarshal(newsHostsJSON, &hosts); err != nil {
		panic("citation: invalid news_hosts.json: " + err.Error())
	}
	set := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		set[h] = true
	}
	return set
})

// IsNewsHost reports whether u is on a well-known news site (or one of its subdomains).
func IsNewsHost(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for host != "" {
		if newsHosts()[host] {
			return true
		}
		i := strings.IndexByte(host, '.')
		if i < 0 {
			break
		}
		host = host[i+1:]
	}
	return false
}
