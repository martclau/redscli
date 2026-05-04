package redscli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// runConfig dispatches local configuration commands.
func (a *app) runConfig(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("config")
		return nil
	}
	switch args[0] {
	case "init":
		return a.runConfigInit(args[1:])
	case "set":
		return a.runConfigSet(args[1:])
	case "show":
		return a.runConfigShow(args[1:])
	case "path":
		fmt.Fprintln(a.out, a.opts.configPath)
		return nil
	default:
		return withExitCode(2, "unknown config subcommand %q", args[0])
	}
}

// runConfigInit creates a new config file with one profile.
func (a *app) runConfigInit(args []string) error {
	fs := newFlagSet("config init")
	token := fs.String("token", "", "API token")
	baseURL := fs.String("base-url", defaultBaseURL, "API base URL")
	profile := fs.String("profile", "default", "profile name")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := requireNoArgs(fs, "config init"); err != nil {
		return err
	}
	cfg := configFile{
		DefaultProfile: *profile,
		Profiles: map[string]profileConfig{
			*profile: {
				Token:   *token,
				BaseURL: *baseURL,
			},
		},
	}
	if err := saveConfig(a.opts.configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "wrote %s\n", a.opts.configPath)
	return nil
}

// runConfigSet updates one setting in the selected profile.
func (a *app) runConfigSet(args []string) error {
	if len(args) == 0 {
		return withExitCode(2, "config set requires token or base-url")
	}
	key := args[0]
	fs := newFlagSet("config set " + key)
	profileFlag := fs.String("profile", "", "profile name")
	if err := parseFlags(fs, args[1:]); err != nil {
		return err
	}
	value, err := requireOneArg(fs, "config set "+key, "value")
	if err != nil {
		return err
	}
	cfg, err := loadConfig(a.opts.configPath)
	if err != nil {
		return err
	}
	profileName := selectedProfile(cfg, *profileFlag)
	profile := ensureProfile(&cfg, profileName)
	switch key {
	case "token":
		profile.Token = value
	case "base-url":
		profile.BaseURL = value
	default:
		return withExitCode(2, "unknown config key %q; use token or base-url", key)
	}
	cfg.Profiles[profileName] = profile
	cfg.DefaultProfile = profileName
	if err := saveConfig(a.opts.configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "updated %s profile %q\n", a.opts.configPath, profileName)
	return nil
}

// runConfigShow prints stored config, redacting tokens by default.
func (a *app) runConfigShow(args []string) error {
	fs := newFlagSet("config show")
	redact := fs.Bool("redact", true, "redact tokens")
	check := fs.Bool("check", false, "also call /quota to verify the effective token")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := requireNoArgs(fs, "config show"); err != nil {
		return err
	}
	cfg, err := loadConfig(a.opts.configPath)
	if err != nil {
		return err
	}
	if *redact {
		for name, profile := range cfg.Profiles {
			profile.Token = redactToken(profile.Token)
			cfg.Profiles[name] = profile
		}
	}
	view := map[string]any{
		"path":            a.opts.configPath,
		"default_profile": cfg.DefaultProfile,
		"profiles":        cfg.Profiles,
	}
	body, err := marshalCompact(view)
	if err != nil {
		return err
	}
	if err := writeBody(a.out, body, a.opts.format); err != nil {
		return err
	}
	if *check {
		if strings.EqualFold(a.opts.format, "raw") {
			fmt.Fprintln(a.errOut, "checking /quota")
		}
		return a.requestJSON(context.Background(), http.MethodGet, "/quota", nil, nil, "", nil)
	}
	return nil
}
