package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ServerConfig struct {
	Address             string           `json:"address"`
	Port                uint16           `json:"port"`
	Domain              string           `json:"domain"`
	EnableSsl           bool             `json:"enable_ssl"`
	EnableSubs          bool             `json:"enable_subs"`
	EnableShell         bool             `json:"enable_shell"`
	EnableYtdlp         bool             `json:"enable_ytdlp"`
	BehindProxy         bool             `json:"behind_proxy"`
	Redirects           []RedirectConfig `json:"redirects"`
	BlacklistedIpRanges [][]string       `json:"blacklisted_ip_ranges"`
	ProfilerOutput      string           `json:"profiler_output"`
}

type RedirectConfig struct {
	Port     uint16 `json:"port"`
	Path     string `json:"path"`
	Location string `json:"location"`
}

type LoggingConfig struct {
	Enabled      bool   `json:"enabled"`
	EnableColors bool   `json:"enable_colors"`
	LogLevel     string `json:"log_level"`
	SaveToFile   bool   `json:"save_to_file"`
	LogDirectory string `json:"log_directory"`
}

type DatabaseConfig struct {
	Enabled  bool   `json:"enabled"`
	Address  string `json:"address"`
	Port     uint16 `json:"port"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type YtdlpConfig struct {
	Enabled        bool   `json:"enabled"`
	EnableServer   bool   `json:"enable_server"`
	ServerAddress  string `json:"server_address"`
	ServerPort     uint16 `json:"server_port"`
	EnableFallback bool   `json:"enable_fallback"`
	FallbackPath   string `json:"fallback_path"`
}

type Config struct {
	Server   ServerConfig   `json:"server"`
	Logging  LoggingConfig  `json:"logging"`
	Database DatabaseConfig `json:"database"`
	Ytdlp    YtdlpConfig    `json:"ytdlp"`
}

func createDefaultConfig() Config {
	server := ServerConfig{
		Address:             "localhost",
		Port:                1234,
		Domain:              "example.com",
		EnableSsl:           false,
		EnableSubs:          false,
		EnableShell:         true,
		EnableYtdlp:         true,
		BehindProxy:         false,
		Redirects:           []RedirectConfig{},
		BlacklistedIpRanges: [][]string{},
		ProfilerOutput:      "",
	}

	logging := LoggingConfig{
		Enabled:      true,
		EnableColors: true,
		LogLevel:     LogLevelToString(LOG_DEBUG),
		SaveToFile:   false,
		LogDirectory: "logs/",
	}

	database := DatabaseConfig{
		Enabled:  false,
		Address:  "localhost",
		Port:     5432,
		Name:     "debug_db",
		Username: "debug_user",
		Password: "debugdb123",
	}

	ytdlp := YtdlpConfig{
		Enabled:        false,
		EnableServer:   true,
		ServerAddress:  "localhost",
		ServerPort:     2345,
		EnableFallback: true,
		FallbackPath:   "yt-dlp",
	}

	config := Config{
		Server:   server,
		Logging:  logging,
		Database: database,
		Ytdlp:    ytdlp,
	}

	return config
}

func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	return true
}

func LoadConfig(config *Config, path string) (bool, string) {
	temp := Config{}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			programPath := GetRelativeExecutablePath()
			return false, fmt.Sprintf("Specified config file does not exist. You can create it by running '%v --generate-config --config-path %v'", programPath, path)
		} else {
			errorMessage := err.(*fs.PathError).Err
			return false, fmt.Sprintf("Failed to open '%v' config file: %v.", path, errorMessage)
		}
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("Failed to read '%v' config file: %v.", path, err.Error())
	}

	err = json.Unmarshal(bytes, &temp)
	if err != nil {
		return false, fmt.Sprintf("Failed deserialize '%v' json config data: %v.", path, err.Error())
	}

	*config = temp
	return true, ""
}

func SaveConfig(config Config, path string) bool {
	file, err := os.Create(path)
	if err != nil {
		errorMessage := err.(*fs.PathError).Err
		fmt.Fprintf(os.Stderr, "ERROR: Failed to create config file '%v': %v.\n", path, errorMessage)
		return false
	}

	defer file.Close()

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to serialize config file '%v': %v.\n", path, err.Error())
		return false
	}

	written, err := file.Write(data)
	if err != nil || written != len(data) {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to write config to the file '%v': %v.\n", path, err.Error())
		return false
	}

	return true
}

func VerifyInputFlags(flags InputFlags) bool {
	if flags.DisableSql && flags.EnableSql {
		fmt.Fprintf(os.Stderr, "ERROR: Both disable and enable SQL flag were provided, but you can only pick one of them.\n")
		return false
	}

	if flags.DisableSsl && flags.EnableSsl {
		fmt.Fprintf(os.Stderr, "ERROR: Both disable and enable SSL flag were provided, but you can only pick one of them.\n")
		return false
	}

	if flags.DisableSubs && flags.EnableSubs {
		fmt.Fprintf(os.Stderr, "ERROR: Both disable and enable subs flag were provided, but you can only pick one of them.\n")
		return false
	}

	if flags.DisableColors && flags.EnableColors {
		fmt.Fprintf(os.Stderr, "ERROR: Both disable and enable colors flag were provided, but you can only pick one of them.\n")
		return false
	}

	if flags.DisableShell && flags.EnableShell {
		fmt.Fprintf(os.Stderr, "ERROR: Both disable and enable interactive shell flags were provided, but you can only pick one of them.\n")
		return false
	}

	if flags.ServerPort > math.MaxUint16 || flags.ServerPort < 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Incorrect port number. Port number must be between values 0 and %v.\n", math.MaxUint16)
		return false
	}

	return true
}

func ApplyInputFlags(config *Config, flags InputFlags) {
	if flags.ServerAddress != "" {
		config.Server.Address = flags.ServerAddress
	}

	if flags.ServerPort != 0 {
		config.Server.Port = uint16(flags.ServerPort)
	}

	if flags.ServerDomain != "" {
		config.Server.Domain = flags.ServerDomain
	}

	if flags.BehindProxy {
		config.Server.BehindProxy = true
	}

	if flags.EnableSql {
		config.Database.Enabled = true
	}

	if flags.EnableSsl {
		config.Server.EnableSsl = true
	}

	if flags.EnableSubs {
		config.Server.EnableSubs = true
	}

	if flags.EnableColors {
		config.Logging.EnableColors = true
	}

	if flags.EnableShell {
		config.Server.EnableShell = true
	}

	if flags.DisableSql {
		config.Database.Enabled = false
	}

	if flags.DisableSsl {
		config.Server.EnableSsl = false
	}

	if flags.DisableSubs {
		config.Server.EnableSubs = false
	}

	if flags.DisableColors {
		config.Logging.EnableColors = false
	}

	if flags.DisableShell {
		config.Server.EnableShell = false
	}

	if flags.ProfileOutput != "" {
		config.Server.ProfilerOutput = flags.ProfileOutput
	}

}

type InputFlags struct {
	ConfigPath     string
	GenerateConfig bool
	ServerAddress  string
	ServerDomain   string
	ServerPort     int
	BehindProxy    bool
	ProfileOutput  string

	EnableSql    bool
	EnableSsl    bool
	EnableSubs   bool
	EnableColors bool
	EnableShell  bool

	DisableSql    bool
	DisableSsl    bool
	DisableSubs   bool
	DisableColors bool
	DisableShell  bool

	ShowHelp bool
}

func nextArg(args *[]string) string {
	if len(*args) == 0 {
		return ""
	}

	arg := (*args)[0]
	*args = (*args)[1:]

	return arg
}

func invalidValue(flag string, value string) bool {
	if value == "" {
		fmt.Fprintf(os.Stderr, "ERROR: Value for the argument %v is missing. See --help for program usage.\n", flag)
		return true
	} else if strings.HasPrefix(value, "--") {
		fmt.Fprintf(os.Stderr, "ERROR: Value for the argument '%v' is missing. Instead a '%v' flag was found. See --help for program usage.\n", flag, value)
		return true
	}

	return false
}

func ParseInputArgs() (InputFlags, bool) {
	args := os.Args
	flags := InputFlags{}

	// Program name
	_ = nextArg(&args)

	arg := nextArg(&args)
	for arg != "" {
		switch arg {
		case "-cp", "--config-path":
			configPath := nextArg(&args)
			if invalidValue(arg, configPath) {
				return flags, false
			}
			flags.ConfigPath = configPath

		case "-gc", "--generate-config":
			flags.GenerateConfig = true

		case "-ip", "--address":
			address := nextArg(&args)
			if invalidValue(arg, address) {
				return flags, false
			}
			flags.ServerAddress = address

		case "-d", "--domain":
			domain := nextArg(&args)
			if invalidValue(arg, domain) {
				return flags, false
			}
			flags.ServerDomain = domain

		case "-p", "--port":
			portArg := nextArg(&args)
			if invalidValue(arg, portArg) {
				return flags, false
			}

			port, err := strconv.Atoi(portArg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to parse number value for the argument '%v'. %v. See --help for program usage.\n", arg, err.Error())
				return flags, false
			}

			flags.ServerPort = port

		case "-prf", "--profile-file":
			outputFile := nextArg(&args)
			if invalidValue(arg, outputFile) {
				return flags, false
			}
			flags.ProfileOutput = outputFile

		case "-bp", "--behind-proxy":
			flags.BehindProxy = true

		case "-h", "--help":
			flags.ShowHelp = true

		// Enable flags":
		case "-sql", "--enable-database":
			flags.EnableSql = true

		case "-ssl", "--enable-encryption":
			flags.EnableSsl = true

		case "-es", "--enable-subs":
			flags.EnableSubs = false

		case "-ec", "--enable-colors":
			flags.EnableColors = true

		case "-esh", "--enable-shell":
			flags.EnableShell = true

		// Disable flags":
		case "-nosql", "--disable-database":
			flags.DisableSql = true

		case "-nossl", "--disable-encryption":
			flags.DisableSsl = true

		case "-ds", "--disable-subs":
			flags.DisableSubs = true

		case "-dc", "--disable-colors":
			flags.DisableColors = true

		case "-dsh", "--disable-shell":
			flags.DisableShell = true

		default:
			fmt.Fprintf(os.Stderr, "ERROR: Input argument '%v' is not valid. See --help for program usage.\n", arg)
			return flags, false
		}

		arg = nextArg(&args)
	}

	return flags, true
}

func GetRelativeExecutablePath() string {
	currentDir, err := os.Getwd()
	if err != nil {
		return "pocketwatch"
	}

	execPath, err := os.Executable()
	if err != nil {
		return "pocketwatch"
	}

	relPath, err := filepath.Rel(currentDir, execPath)
	if err != nil {
		return "pocketwatch"
	}

	return relPath

}

func GetExecutableName() string {
	path, err := os.Executable()
	if err != nil {
		return "pocketwatch"
	}

	path = filepath.Base(path)
	return path
}

const VERSION string = "0.0.1-alpha"

func DisplayHelp() {
	exe := GetExecutableName()

	fmt.Println()
	fmt.Println("Pocket Watch ", "v"+VERSION)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("    ", exe, "[OPTIONS]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("    -h,     --help                   Display this help message.")
	fmt.Println("    -cp,    --config-path [path]     Loads config from the provided path.")
	fmt.Println("    -gc,    --generate-config        Generates default config. Can also be use in combination with --config-path to specify output path. (default: ./config.json)")
	fmt.Println("    -ip,    --address [10.0.0.1]     Binds server to an address. (default: localhost)")
	fmt.Println("    -p,     --port [443]             Set address port to bind. (values between '0-65535') (default: 1234)")
	fmt.Println("    -bp,    --behind-proxy           Changes the server behavior to rely on HTTP headers such as X-Real-Ip for rate-limiting (default: false)")
	fmt.Println("    -sql,   --enable-database        Enables support for the Postgres SQL database persistence. (default: disabled)")
	fmt.Println("    -ssl,   --enable-encryption      Enables encrypted connection between a defaultClient and the server. Secrets are read from:")
	fmt.Println("                                       - CERTIFICATE: ./secret/certificate.pem")
	fmt.Println("                                       - PRIVATE KEY: ./secret/privatekey.pem")
	fmt.Println("    -es,    --enable-subs            Enables support for subtitle search. (default: disabled)")
	fmt.Println("    -ec,    --enable-color           Enables colored logging.")
	fmt.Println("    -esh,   --enable-shell           Enables interactive shell during server runtime.")
	fmt.Println("    -nosql, --disable-database       Disables support for the Postgres SQL database persistence.")
	fmt.Println("    -nossl, --disable-encryption     Disables encrypted connection between a client and the server.")
	fmt.Println("    -ds,    --disable-subs           Disables support for subtitle search.")
	fmt.Println("    -dc,    --disable-color          Disables colored logging. (default: enabled)")
	fmt.Println("    -d,     --domain [example.com]   Domain, if any, that the server is hosted on. Serves as a hint to associate URLs with local env.")
	fmt.Println("    -dsh,   --disable-shell          Disable interactive shell during server runtime.")
	fmt.Println("    -prf,   --profiler-file [file]   Output file for the recorded data from the profiler (disabled by default).")
	fmt.Println()
	fmt.Println("Example usage:")
	fmt.Println("    ", exe, "--port 8888")
}

func PrettyPrintConfig(config Config) {

	const ESC = "\033["
	const RESET = ESC + "m"
	const FG_RED = ESC + "1;31m"
	const FG_GREEN = ESC + "1;32m"
	const FG_PURPLE = ESC + "1;35m"

	boolColor := func(value bool) string {
		if !config.Logging.EnableColors {
			return fmt.Sprint(value)
		}

		var color string
		if value {
			color = FG_GREEN
		} else {
			color = FG_RED
		}

		return fmt.Sprintf("%s%v%s", color, value, RESET)
	}

	purpleColor := func(value any) string {
		if !config.Logging.EnableColors {
			return fmt.Sprint(value)
		}

		return fmt.Sprintf("%s%v%s", FG_PURPLE, value, RESET)
	}

	headers := []string{
		purpleColor("ip"),
		purpleColor("port"),
		purpleColor("domain"),
		purpleColor("ssl"),
		purpleColor("shell"),
		purpleColor("ytdlp"),
		purpleColor("subs"),
		purpleColor("proxied"),
		purpleColor("database"),
	}

	values := []string{
		fmt.Sprint(config.Server.Address),
		fmt.Sprint(config.Server.Port),
		fmt.Sprint(config.Server.Domain),
		boolColor(config.Server.EnableSsl),
		boolColor(config.Server.EnableShell),
		boolColor(config.Server.EnableYtdlp),
		boolColor(config.Server.EnableSubs),
		boolColor(config.Server.BehindProxy),
		boolColor(config.Database.Enabled),
	}

	table := GeneratePrettyVerticalTable("Server Config", headers, values)
	fmt.Print(table)

	headers = []string{
		purpleColor("enabled"),
		purpleColor("colors"),
		purpleColor("level"),
		purpleColor("save logs"),
		purpleColor("output dir"),
	}

	values = []string{
		boolColor(config.Logging.Enabled),
		boolColor(config.Logging.EnableColors),
		fmt.Sprint(config.Logging.LogLevel),
		boolColor(config.Logging.SaveToFile),
		fmt.Sprint(config.Logging.LogDirectory),
	}

	table = GeneratePrettyVerticalTable("Logging Config", headers, values)
	fmt.Print(table)

	headers = []string{
		purpleColor("enabled"),
		purpleColor("use server"),
		purpleColor("address"),
		purpleColor("port"),
		purpleColor("use cli"),
		purpleColor("ytdlp path"),
	}

	values = []string{
		boolColor(config.Ytdlp.Enabled),
		boolColor(config.Ytdlp.EnableServer),
		fmt.Sprint(config.Ytdlp.ServerAddress),
		fmt.Sprint(config.Ytdlp.ServerPort),
		boolColor(config.Ytdlp.EnableFallback),
		fmt.Sprint(config.Ytdlp.FallbackPath),
	}

	table = GeneratePrettyVerticalTable("Ytdlp Config", headers, values)
	fmt.Print(table)
}
