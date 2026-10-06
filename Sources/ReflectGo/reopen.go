package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type UISession struct {
	Base  string `json:"base_url"`
	Token string `json:"token"`
}

func validateUISession(s UISession) error {
	u, err := url.Parse(s.Base)
	if err != nil {
		return err
	}
	p, e := strconv.Atoi(u.Port())
	raw, d := hex.DecodeString(s.Token)
	if e != nil || p < 1024 || p > 65535 || s.Base != fmt.Sprintf("http://127.0.0.1:%d", p) || d != nil || len(raw) != 32 {
		return errors.New("invalid workstation reconnect record")
	}
	return nil
}

func reopenUI(root string) error {
	path, err := statePath(root, "browser-session.json")
	if err != nil {
		return err
	}
	// A second click may arrive while the first process is still creating the server.
	for i := 0; i < 30; i++ {
		var s UISession
		if readJSON(path, &s) == nil && validateUISession(s) == nil {
			req, _ := http.NewRequest("GET", s.Base+"/api/status", nil)
			req.Header.Set("Authorization", "Bearer "+s.Token)
			client := *localHTTP
			client.Timeout = time.Second
			if r, e := client.Do(req); e == nil {
				io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
				r.Body.Close()
				if r.StatusCode == 200 {
					if os.Getenv("REFLECT_GO_NO_BROWSER") != "1" {
						if e = openBrowser(s.Base + "/#" + s.Token); e != nil {
							return fmt.Errorf("workstation is running; open state/OPEN-WORKSTATION.html: %w", e)
						}
					}
					fmt.Println("Reopened your running Reflect Go workstation. No second engine was started.")
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the workstation is starting or its folder is locked. Wait a few seconds and open START Reflect Go again; if it still fails, close its existing launcher first")
}
