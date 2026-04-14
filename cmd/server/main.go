package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cmd184psu/grocery-list/internal/api"
	"github.com/cmd184psu/grocery-list/internal/config"
	"github.com/cmd184psu/grocery-list/internal/store"
)

func main() {
	cfgPath     := flag.String("config",     config.DefaultConfigPath, "Path to config JSON file")
	flagPort    := flag.Int("port",          0,  "Override port from config")
	flagCert    := flag.String("tls-cert",   "", "Override TLS cert path")
	flagKey     := flag.String("tls-key",    "", "Override TLS key path")
	flagWeb     := flag.String("web-dir",    "", "Override static web directory")
	flagData    := flag.String("data-file",  "", "Override data file path")
	flagInitCfg := flag.Bool("init-config", false, "Write default config and exit")
	flag.Parse()

	if *flagInitCfg {
		if err := config.WriteDefault(*cfgPath); err != nil {
			log.Fatalf("Failed to write default config: %v", err)
		}
		fmt.Printf("Default config written to %s\n", *cfgPath)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	// CLI flag overrides
	if *flagPort != 0  { cfg.Port      = *flagPort }
	if *flagCert != "" { cfg.TLSCert   = *flagCert }
	if *flagKey  != "" { cfg.TLSKey    = *flagKey  }
	if *flagWeb  != "" { cfg.StaticDir = *flagWeb  }
	if *flagData != "" { cfg.DataFile  = *flagData }

	// Ensure data directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.DataFile), 0755); err != nil {
		log.Fatalf("Cannot create data directory: %v", err)
	}

	s, err := store.New(cfg.DataFile)
	if err != nil {
		log.Fatalf("Failed to init store: %v", err)
	}

	// Merge persisted groups with config groups (config wins for ordering if both present)
	persistedGroups := s.Groups()
	effectiveGroups := cfg.Groups
	if len(effectiveGroups) == 0 {
		effectiveGroups = persistedGroups
	}

	h   := api.NewHandler(s, effectiveGroups)
	mux := http.NewServeMux()
	h.Register(mux)

	// Static file server with index.html fallback for SPA-style routing
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(cfg.StaticDir, filepath.Clean("/"+r.URL.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "index.html"))
			return
		}
		http.ServeFile(w, r, path)
	})

	handler := api.Wrap(mux)
	addr    := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	useTLS  := cfg.TLSCert != "" && cfg.TLSKey != ""

	if useTLS {
		log.Printf("Grocery List → https://%s  (TLS)", addr)
		log.Fatalf("HTTPS error: %v", http.ListenAndServeTLS(addr, cfg.TLSCert, cfg.TLSKey, handler))
	} else {
		log.Printf("Grocery List → http://%s", addr)
		log.Fatalf("HTTP error: %v", http.ListenAndServe(addr, handler))
	}
}
