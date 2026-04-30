package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ScrapeTimeout  Duration                `yaml:"scrape_timeout"`
	ConnectTimeout Duration                `yaml:"connect_timeout"`
	Collectors     CollectorSet            `yaml:"collectors"`
	Targets        map[string]TargetConfig `yaml:"targets"`
}

type TargetConfig struct {
	Host          string            `yaml:"host"`
	Port          int               `yaml:"port"`
	Service       string            `yaml:"service"`
	ConnectString string            `yaml:"connect_string"`
	Username      string            `yaml:"username"`
	Password      string            `yaml:"password"`
	Privilege     string            `yaml:"privilege"` // sysdba, sysasm, sysoper — required for ASM
	Labels        map[string]string `yaml:"labels"`
	Collectors    CollectorOverrides `yaml:"collectors"`
}

func (t TargetConfig) UseWallet() bool {
	return t.Username == "" && t.Password == ""
}

// CollectorOverride is a per-target override for a single collector.
// In YAML it accepts either a bare bool (enable/disable shorthand) or a struct
// with optional enabled/include/exclude fields that replace the global values.
//
//	asm_diskgroup: true                  # shorthand — just enable
//	sysstat:                             # full override
//	  enabled: true
//	  include: ["^user commits$"]
type CollectorOverride struct {
	Enabled *bool
	Include []string
	Exclude []string
}

func (c *CollectorOverride) UnmarshalYAML(value *yaml.Node) error {
	var b bool
	if err := value.Decode(&b); err == nil {
		c.Enabled = &b
		return nil
	}
	var s struct {
		Enabled *bool    `yaml:"enabled"`
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	}
	if err := value.Decode(&s); err != nil {
		return err
	}
	c.Enabled = s.Enabled
	c.Include = s.Include
	c.Exclude = s.Exclude
	return nil
}

// CollectorOverrides holds optional per-target collector overrides.
// A nil pointer means "inherit from global default".
type CollectorOverrides struct {
	Tablespace        *CollectorOverride `yaml:"tablespace"`
	ASMDiskgroup      *CollectorOverride `yaml:"asm_diskgroup"`
	Session           *CollectorOverride `yaml:"session"`
	Sysstat           *CollectorOverride `yaml:"sysstat"`
	SysWaitClass      *CollectorOverride `yaml:"syswaitclass"`
	SysTimeModel      *CollectorOverride `yaml:"systimemodel"`
	Event             *CollectorOverride `yaml:"event"`
	FlashRecoveryArea *CollectorOverride `yaml:"flash_recovery_area"`
	Uptime            *CollectorOverride `yaml:"uptime"`
	Dataguard         *CollectorOverride `yaml:"dataguard"`
	Sanity            *CollectorOverride `yaml:"sanity"`
}

// Apply merges per-target overrides on top of the global CollectorSet.
// Fields present in the override replace the global value; absent fields inherit it.
func (o CollectorOverrides) Apply(base CollectorSet) CollectorSet {
	apply := func(override *CollectorOverride, cfg CollectorConfig) CollectorConfig {
		if override == nil {
			return cfg
		}
		if override.Enabled != nil {
			cfg.Enabled = override.Enabled
		}
		if override.Include != nil {
			cfg.Include = override.Include
		}
		if override.Exclude != nil {
			cfg.Exclude = override.Exclude
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
		ScrapeTimeout:  Duration{30 * time.Second},
		ConnectTimeout: Duration{10 * time.Second},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("no targets defined")
	}
	for name, t := range cfg.Targets {
		if t.ConnectString == "" && (t.Host == "" || t.Service == "") {
			return nil, fmt.Errorf("target %q: connect_string or host+service required", name)
		}
		if !t.UseWallet() && t.Password == "" {
			return nil, fmt.Errorf("target %q: password is required when username is set", name)
		}
	}
	return cfg, nil
}
