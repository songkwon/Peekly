package capture

import "testing"

func TestRegistrableDomainInSnapshots(t *testing.T) {
	cases := map[string]string{
		"api.example.com":       "example.com",
		"img.shop.example.com.": "example.com",
		"api.Example.CO.UK":     "example.co.uk",
		"api.alice.github.io":   "alice.github.io",
		"www.例子.中国":             "xn--fsqu00a.xn--fiqs8s",
		"localhost":             "localhost",
		"192.0.2.15":            "192.0.2.15",
		"2001:db8::1":           "2001:db8::1",
	}
	store := newStore()
	for host, want := range cases {
		row := Row{Host: host}
		id := store.insert(row)
		store.update(id, Detail{Row: row})
		detail, _ := store.get(id)
		if detail.Domain != want || detail.Host != host {
			t.Errorf("host %q: domain = %q, host = %q, want domain %q", host, detail.Domain, detail.Host, want)
		}
	}
}
