package openai

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	webSocketCookieMaxHosts       = 128
	webSocketCookieMaxPerHost     = 32
	webSocketCookieMaxNameBytes   = 128
	webSocketCookieMaxValueBytes  = 4096
	webSocketCookieMaxPathBytes   = 512
	webSocketCookieMaxHeaderBytes = 1 << 20
)

type webSocketCookieKey struct {
	profile string
	host    string
}

type webSocketCookieID struct {
	name string
	path string
}

type webSocketCookie struct {
	name      string
	value     string
	path      string
	secure    bool
	expiresAt time.Time
	updatedAt time.Time
}

type webSocketCookieChange struct {
	id     webSocketCookieID
	cookie webSocketCookie
	delete bool
}

type webSocketCookieJar struct {
	mu    sync.Mutex
	hosts map[webSocketCookieKey]map[webSocketCookieID]webSocketCookie
}

func newWebSocketCookieJar() *webSocketCookieJar {
	return &webSocketCookieJar{hosts: make(map[webSocketCookieKey]map[webSocketCookieID]webSocketCookie)}
}

func websocketCookieProfile(account proxymodel.Account) string {
	if profile := strings.TrimSpace(account.ID); profile != "" {
		return profile
	}
	return account.Home
}

func (jar *webSocketCookieJar) capture(profile string, target *url.URL, headers http.Header) {
	host := webSocketCookieHost(target)
	if profile == "" || host == "" {
		return
	}
	now := time.Now()
	defaultPath := webSocketCookieDefaultPath(target.EscapedPath())
	var changes []webSocketCookieChange
	for _, value := range headers.Values("Set-Cookie") {
		if change, ok := parseWebSocketSetCookie(value, defaultPath, webSocketCookieSecureURL(target), now); ok {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return
	}

	key := webSocketCookieKey{profile: profile, host: host}
	jar.mu.Lock()
	defer jar.mu.Unlock()
	jar.pruneExpired(now)
	for _, change := range changes {
		cookies := jar.hosts[key]
		if cookies == nil {
			cookies = make(map[webSocketCookieID]webSocketCookie)
			jar.hosts[key] = cookies
		}
		if change.delete {
			delete(cookies, change.id)
		} else {
			cookies[change.id] = change.cookie
		}
		jar.pruneHost(cookies)
		if len(cookies) == 0 {
			delete(jar.hosts, key)
		}
	}
	jar.pruneHosts()
}

func (jar *webSocketCookieJar) header(profile string, target *url.URL, headers http.Header) string {
	segments, callerNames := webSocketCallerCookies(headers)
	host := webSocketCookieHost(target)
	if profile == "" || host == "" {
		return strings.Join(segments, "; ")
	}

	now := time.Now()
	key := webSocketCookieKey{profile: profile, host: host}
	jar.mu.Lock()
	defer jar.mu.Unlock()
	jar.pruneExpired(now)
	matching := make([]webSocketCookie, 0)
	if cookies := jar.hosts[key]; cookies != nil {
		for id, cookie := range cookies {
			if _, callerWins := callerNames[id.name]; callerWins || cookie.secure && !webSocketCookieSecureURL(target) || !webSocketCookiePathMatches(target.EscapedPath(), cookie.path) {
				continue
			}
			matching = append(matching, cookie)
		}
	}
	sort.Slice(matching, func(i, j int) bool {
		if len(matching[i].path) != len(matching[j].path) {
			return len(matching[i].path) > len(matching[j].path)
		}
		return matching[i].name < matching[j].name
	})
	for _, cookie := range matching {
		segments = append(segments, cookie.name+"="+cookie.value)
	}
	return strings.Join(segments, "; ")
}

func (jar *webSocketCookieJar) clear() {
	jar.mu.Lock()
	jar.hosts = make(map[webSocketCookieKey]map[webSocketCookieID]webSocketCookie)
	jar.mu.Unlock()
}

func webSocketCallerCookies(headers http.Header) ([]string, map[string]struct{}) {
	var segments []string
	names := make(map[string]struct{})
	for _, value := range headers.Values("Cookie") {
		for _, segment := range strings.Split(value, ";") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			segments = append(segments, segment)
			if name, _, ok := strings.Cut(segment, "="); ok {
				name = strings.TrimSpace(name)
				if validWebSocketCookieName(name) {
					names[name] = struct{}{}
				}
			}
		}
	}
	return segments, names
}

func parseWebSocketSetCookie(header, defaultPath string, secureOrigin bool, now time.Time) (webSocketCookieChange, bool) {
	if len(header) > webSocketCookieMaxHeaderBytes {
		return webSocketCookieChange{}, false
	}
	parts := strings.Split(header, ";")
	first := strings.TrimSpace(parts[0])
	equals := strings.IndexByte(first, '=')
	if equals < 0 {
		return webSocketCookieChange{}, false
	}
	name := strings.TrimSpace(first[:equals])
	value := strings.TrimSpace(first[equals+1:])
	if !validWebSocketCookieName(name) || !validWebSocketCookieValue(value) {
		return webSocketCookieChange{}, false
	}

	path := defaultPath
	secure := false
	var expiresAt time.Time
	deleteCookie := false
	maxAgeSeen := false
	for _, attribute := range parts[1:] {
		attribute = strings.TrimSpace(attribute)
		if strings.EqualFold(attribute, "secure") {
			secure = true
			continue
		}
		attributeName, attributeValue, hasValue := strings.Cut(attribute, "=")
		if !hasValue {
			continue
		}
		attributeName = strings.TrimSpace(attributeName)
		attributeValue = strings.TrimSpace(attributeValue)
		switch {
		case strings.EqualFold(attributeName, "path"):
			if validWebSocketCookiePath(attributeValue) {
				path = attributeValue
			}
		case strings.EqualFold(attributeName, "max-age"):
			if strings.HasPrefix(attributeValue, "+") {
				continue
			}
			seconds, err := strconv.ParseInt(attributeValue, 10, 64)
			if err != nil {
				continue
			}
			maxAgeSeen = true
			deleteCookie = seconds <= 0
			if seconds > 0 {
				expiresAt = webSocketCookieMaxAgeExpiry(now, seconds)
			}
		case !maxAgeSeen && strings.EqualFold(attributeName, "expires"):
			if expires, err := http.ParseTime(attributeValue); err == nil {
				expiresAt = expires
				deleteCookie = !expires.After(now)
			}
		}
	}
	if secure && !secureOrigin {
		return webSocketCookieChange{}, false
	}
	id := webSocketCookieID{name: name, path: path}
	return webSocketCookieChange{
		id: id,
		cookie: webSocketCookie{
			name: name, value: value, path: path, secure: secure, expiresAt: expiresAt, updatedAt: now,
		},
		delete: deleteCookie,
	}, true
}

func webSocketCookieMaxAgeExpiry(now time.Time, seconds int64) time.Time {
	maxUnixSeconds := int64(^uint64(0) >> 1)
	if seconds > maxUnixSeconds-now.Unix() {
		return now
	}
	return time.Unix(now.Unix()+seconds, int64(now.Nanosecond()))
}

func validWebSocketCookieName(name string) bool {
	if name == "" || len(name) > webSocketCookieMaxNameBytes {
		return false
	}
	for _, value := range []byte(name) {
		if !(value == '!' || value >= '#' && value <= '\'' || value >= '*' && value <= '+' ||
			value >= '-' && value <= '.' || value >= '0' && value <= '9' ||
			value >= 'A' && value <= 'Z' || value >= '^' && value <= 'z' ||
			value == '|' || value == '~') {
			return false
		}
	}
	return true
}

func validWebSocketCookieValue(value string) bool {
	if len(value) > webSocketCookieMaxValueBytes {
		return false
	}
	for _, character := range []byte(value) {
		if !(character == 0x21 || character >= 0x23 && character <= 0x2b ||
			character >= 0x2d && character <= 0x3a || character >= 0x3c && character <= 0x5b ||
			character >= 0x5d && character <= 0x7e) {
			return false
		}
	}
	return true
}

func validWebSocketCookiePath(path string) bool {
	if path == "" || len(path) > webSocketCookieMaxPathBytes || path[0] != '/' {
		return false
	}
	return !strings.ContainsAny(path, "\r\n")
}

func webSocketCookieDefaultPath(path string) string {
	if path == "" || path[0] != '/' {
		return "/"
	}
	if slash := strings.LastIndexByte(path, '/'); slash > 0 {
		return path[:slash]
	}
	return "/"
}

func webSocketCookiePathMatches(requestPath, cookiePath string) bool {
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	return len(requestPath) == len(cookiePath) || strings.HasSuffix(cookiePath, "/") || requestPath[len(cookiePath)] == '/'
}

func webSocketCookieHost(target *url.URL) string {
	if target == nil {
		return ""
	}
	return strings.Trim(strings.ToLower(target.Hostname()), ".")
}

func webSocketCookieSecureURL(target *url.URL) bool {
	return target != nil && (strings.EqualFold(target.Scheme, "https") || strings.EqualFold(target.Scheme, "wss"))
}

func (jar *webSocketCookieJar) pruneExpired(now time.Time) {
	for key, cookies := range jar.hosts {
		for id, cookie := range cookies {
			if !cookie.expiresAt.IsZero() && !cookie.expiresAt.After(now) {
				delete(cookies, id)
			}
		}
		if len(cookies) == 0 {
			delete(jar.hosts, key)
		}
	}
}

func (jar *webSocketCookieJar) pruneHost(cookies map[webSocketCookieID]webSocketCookie) {
	for len(cookies) > webSocketCookieMaxPerHost {
		var oldestID webSocketCookieID
		var oldest webSocketCookie
		first := true
		for id, cookie := range cookies {
			if first || cookie.updatedAt.Before(oldest.updatedAt) || cookie.updatedAt.Equal(oldest.updatedAt) && (id.name < oldestID.name || id.name == oldestID.name && id.path < oldestID.path) {
				oldestID, oldest, first = id, cookie, false
			}
		}
		delete(cookies, oldestID)
	}
}

func (jar *webSocketCookieJar) pruneHosts() {
	for len(jar.hosts) > webSocketCookieMaxHosts {
		var oldestKey webSocketCookieKey
		var oldest time.Time
		first := true
		for key, cookies := range jar.hosts {
			for _, cookie := range cookies {
				if first || cookie.updatedAt.Before(oldest) || cookie.updatedAt.Equal(oldest) && (key.profile < oldestKey.profile || key.profile == oldestKey.profile && key.host < oldestKey.host) {
					oldestKey, oldest, first = key, cookie.updatedAt, false
				}
			}
		}
		delete(jar.hosts, oldestKey)
	}
}
