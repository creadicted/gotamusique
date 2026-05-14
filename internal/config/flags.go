package config

import "github.com/spf13/pflag"

// ParseFlags registers CLI flags, parses os.Args, and returns the user config
// path (empty if --config was not set) and an apply function that overwrites
// cfg fields for any flag that was explicitly provided.
func ParseFlags() (userPath string, apply func(*Config)) {
	pflag.String("config", "", "path to user configuration file")
	pflag.StringP("server", "s", "", "Mumble server hostname")
	pflag.StringP("username", "u", "", "bot username")
	pflag.StringP("password", "P", "", "server password")
	pflag.IntP("port", "p", 0, "server port")
	pflag.StringP("certificate", "c", "", "TLS client certificate file")
	pflag.StringP("channel", "C", "", "channel to join on connect")
	pflag.IntP("bandwidth", "b", 0, "audio bandwidth in bps")
	pflag.Parse()

	userPath, _ = pflag.CommandLine.GetString("config")

	apply = func(cfg *Config) {
		fs := pflag.CommandLine
		if fs.Changed("server") {
			v, _ := fs.GetString("server")
			cfg.Server.Host = v
		}
		if fs.Changed("username") {
			v, _ := fs.GetString("username")
			cfg.Bot.Username = v
		}
		if fs.Changed("password") {
			v, _ := fs.GetString("password")
			cfg.Server.Password = v
		}
		if fs.Changed("port") {
			v, _ := fs.GetInt("port")
			cfg.Server.Port = v
		}
		if fs.Changed("certificate") {
			v, _ := fs.GetString("certificate")
			cfg.Server.Certificate = v
		}
		if fs.Changed("channel") {
			v, _ := fs.GetString("channel")
			cfg.Server.Channel = v
		}
		if fs.Changed("bandwidth") {
			v, _ := fs.GetInt("bandwidth")
			cfg.Bot.Bandwidth = v
		}
	}
	return
}
