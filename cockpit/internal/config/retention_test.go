package config

import (
	"testing"
	"time"
)

// L1 — Pre-7: la retention della cache dei contenuti ([retention].cache_gg, cache_max_mb).
//
// Tre regole: assente vale trenta giorni; `giorni_staging`, la voce di prima del blocco 7, vale come
// `cache_gg` se `cache_gg` manca (chi l'aveva scritta aveva gia' deciso, e non deve riscriverlo);
// zero spegne la rimozione per eta' e lascia solo la capienza.

func retentionDa(t *testing.T, corpo string) *Config {
	t.Helper()
	c, err := Carica(scriviCon(t, "", "", corpo))
	if err != nil {
		t.Fatalf("configurazione rifiutata: %v", err)
	}
	return c
}

func TestLaRetentionDellaCacheValeTrentaGiorniSeNonSiDiceNiente(t *testing.T) {
	c := retentionDa(t, "")
	if c.RetentionCache() != 30*24*time.Hour {
		t.Errorf("retention = %s, attesi 30 giorni", c.RetentionCache())
	}
	if c.CacheMaxByte() != 0 {
		t.Errorf("capienza = %d, attesa illimitata", c.CacheMaxByte())
	}
}

func TestGiorniStagingValeComeCacheGgSeCacheGgManca(t *testing.T) {
	c := retentionDa(t, "[retention]\ngiorni_staging = 90\n")
	if c.RetentionCache() != 90*24*time.Hour {
		t.Errorf("retention = %s, attesi 90 giorni da giorni_staging", c.RetentionCache())
	}
}

func TestCacheGgVinceSuGiorniStaging(t *testing.T) {
	c := retentionDa(t, "[retention]\ngiorni_staging = 90\ncache_gg = 15\n")
	if c.RetentionCache() != 15*24*time.Hour {
		t.Errorf("retention = %s, attesi 15 giorni da cache_gg", c.RetentionCache())
	}
}

func TestCacheGgZeroSpegneLaRimozionePerEta(t *testing.T) {
	c := retentionDa(t, "[retention]\ngiorni_staging = 90\ncache_gg = 0\ncache_max_mb = 512\n")
	if c.RetentionCache() != 0 {
		t.Errorf("retention = %s, atteso zero (mai per eta')", c.RetentionCache())
	}
	if c.CacheMaxByte() != 512<<20 {
		t.Errorf("capienza = %d, attesi 512 MB", c.CacheMaxByte())
	}
}
