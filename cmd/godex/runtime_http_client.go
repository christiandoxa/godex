package main

import "net/http"

func runtimeUpstreamHTTPClient(noProxy bool) *http.Client {
	if !noProxy {
		return nil
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Transport: http.DefaultTransport}
	}
	clone := transport.Clone()
	clone.Proxy = nil
	return &http.Client{Transport: clone}
}
