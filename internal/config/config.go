package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ScrapeTimeout Duration               `yaml:"scrape_timeout"`
	Collectors    CollectorSet           `yaml:"collectors"`
	Targets       map[string]TargetConfig `yaml:"targets"`
}

type TargetConfig struct {
	Host           string           `yaml:"host"`
	Port           int              `yaml:"port"`
	Service        string           `yaml:"service"`
	ConnectString  string           `yaml:"connect_string"`
	Username       string           `yaml:"username"`
	Password       string           `yaml:"password"`
	WalletLocation string           `yaml:"wallet_location"`
	Privilege      string           `yaml:"privilege"` // sysdba, sysasm, sysoper — required for ASM
	Collectors     CollectorOverrides `yaml:"collectors"`
}

// CollectorOverrides holds optional per-target collector switches.
// A nil pointer means "inherit from global default".
type CollectorOverrides struct {
	Tablespace        *bool `yaml:"tablespace"`
	ASMDiskgroup      *bool `yaml:"asm_diskgroup"`
	Session           *bool `yaml:"session"`
	Sysstat           *bool `yaml:"sysstat"`
	SysWaitClass      *bool `yaml:"syswaitclass"`
	SysTimeModel      *bool `yaml:"systimemodel"`
	Event             *bool `yaml:"event"`
	FlashRecoveryArea *bool `yaml:"flash_recovery_area"`
	Uptime            *bool `yaml:"uptime"`
	Dataguard         *bool `yaml:"dataguard"`
	Sanity            *bool `yaml:"sanity"`
}

// Apply merges per-target overrides on top of the global CollectorSet.
func (o CollectorOverrides) Apply(base CollectorSet) CollectorSet {
	apply := func(override *bool, cfg CollectorConfig) CollectorConfig {
		if override != nil {
			cfg.Enabled = override
		}
		return cfg
	}
	return CollectorSet{
		Tablespace:        apply(o.Tablespace, base.Tablespace),
		ASMDiskgroup:      apply(o.ASMDiskgroup, base.ASMDiskgroup),
		Session:           apply(o.Session, base.Session),
		Sysstat:           apply(o.Sysstat, base.Sysstat),
		SysWaitClass:      apply(o.SysWaitClass, base.SysWaitClass),
		SysTimeModel:      apply(o.SysTimeModel, base.SysTimeModel),
		Event:             apply(o.Event, base.Event),
		FlashRecoveryArea: apply(o.FlashRecoveryArea, base.FlashRecoveryArea),
		Uptime:            apply(o.Uptime, base.Uptime),
		Dataguard:         apply(o.Dataguard, base.Dataguard),
		Sanity:            apply(o.Sanity, base.Sanity),
	}
}

func (t TargetConfig) DSN() string {
	cs := t.ConnectString
	if cs == "" {
		port := t.Port
		if port == 0 {
			port = 1521
		}
		cs = fmt.Sprintf("%s:%d/%s", t.Host, port, t.Service)
	}
	dsn := fmt.Sprintf(`connectString="%s"`, cs)
	if t.Username != "" {
		dsn = fmt.Sprintf(`user="%s" `, t.Username) + dsn
	}
	if t.Password != "" {
		dsn += fmt.Sprintf(` password="%s"`, t.Password)
	}
	switch t.Privilege {
	case "sysdba":
		dsn += " sysdba=1"
	case "sysoper":
		dsn += " sysoper=1"
	case "sysasm":
		dsn += " sysasm=1"
	}
	if t.WalletLocation != "" {
		dsn += fmt.Sprintf(` walletLocation="%s"`, t.WalletLocation)
	}
	return dsn
}

type CollectorSet struct {
	Tablespace        CollectorConfig `yaml:"tablespace"`
	ASMDiskgroup      CollectorConfig `yaml:"asm_diskgroup"`
	Session           CollectorConfig `yaml:"session"`
	Sysstat           CollectorConfig `yaml:"sysstat"`
	SysWaitClass      CollectorConfig `yaml:"syswaitclass"`
	SysTimeModel      CollectorConfig `yaml:"systimemodel"`
	Event             CollectorConfig `yaml:"event"`
	FlashRecoveryArea CollectorConfig `yaml:"flash_recovery_area"`
	Uptime            CollectorConfig `yaml:"uptime"`
	Dataguard         CollectorConfig `yaml:"dataguard"`
	Sanity            CollectorConfig `yaml:"sanity"`
}

type CollectorConfig struct {
	Enabled *bool    `yaml:"enabled"` // nil = true (default on)
	Include []string `yaml:"include"` // if set, only names matching any pattern are emitted
	Exclude []string `yaml:"exclude"` // names matching any pattern are dropped (after include)
}

func (c CollectorConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// IsExplicitlyEnabled returns true only when enabled is explicitly set to true.
// Used for opt-in collectors (asm_diskgroup on non-ASM targets).
func (c CollectorConfig) IsExplicitlyEnabled() bool {
	return c.Enabled != nil && *c.Enabled
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := &Config{
		ScrapeTimeout: Duration{30 * time.Second},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("no targets defined")
	}
	for name, t := range cfg.Targets {
		if t.Username == "" && t.WalletLocation == "" {
			return nil, fmt.Errorf("target %q: username is required (or set wallet_location)", name)
		}
		if t.Password == "" && t.WalletLocation == "" {
			return nil, fmt.Errorf("target %q: password is required (or set wallet_location)", name)
		}
		if t.ConnectString == "" && (t.Host == "" || t.Service == "") {
			return nil, fmt.Errorf("target %q: connect_string or host+service required", name)
		}
	}
	return cfg, nil
}
