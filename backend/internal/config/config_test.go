package config

import "testing"

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"development without a database is allowed", Config{Env: "development"}, false},
		{"development with demo mode is allowed", Config{Env: "development", DemoMode: true}, false},
		{"production without a database is refused", Config{Env: "production"}, true},
		{"production with demo mode is refused", Config{Env: "production", DatabaseURL: "postgres://x", DemoMode: true}, true},
		{"production, safe settings", Config{Env: "production", DatabaseURL: "postgres://x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.cfg.Validate(); (err != nil) != c.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
