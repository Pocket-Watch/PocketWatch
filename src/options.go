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
	Address      string `json:"address"`
	Port         uint16 `json:"port"`
	RedirectPort uint16 `json:"redirect_port"`
	RedirectTo   string `json:"redirect_to"`
	Domain       string `json:"domain"`
	EnableSsl    bool   `json:"enable_ssl"`
	EnableSubs   bool   `json:"enable_subs"`
	EnableShell  bool   `json:"enable_shell"`
}

type LoggingConfig struct {
	Enabled      bool   `json:"enabled"`
	EnableColors bool   `json:"enable_colors"`
	LogLevel     uint32 `json:"log_level"`
	// NOTE(kihau): Placeholders, also log archiving will be added.
	SaveToFile   bool   `json:"save_to_file"`
	LogFile      string `json:"logfile"`
	LogDirectory string `json:"logdirectory"`
}

type DatabaseConfig struct {
	Enabled  bool   `json:"enabled"`
	Address  string `json:"address"`
	Port     uint16 `json:"port"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	Server   ServerConfig   `json:"server"`
	Logging  LoggingConfig  `json:"logging"`
	Database DatabaseConfig `json:"database"`
}

func createDefaultConfig() Config {
	server := ServerConfig{
		Address:      "localhost",
		Port:         1234,
		RedirectPort: 0,
		Domain:       "example.com",
		EnableSsl:    false,
		EnableSubs:   false,
		EnableShell:  true,
	}

	logging := LoggingConfig{
		Enabled:      true,
		EnableColors: true,
		LogLevel:     LOG_DEBUG,
		SaveToFile:   false,
		LogFile:      "latest.log",
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

	config := Config{
		Server:   server,
		Logging:  logging,
		Database: database,
	}

	return config
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

}

type InputFlags struct {
	ConfigPath     string
	GenerateConfig bool
	ServerAddress  string
	ServerDomain   string
	ServerPort     int

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
	fmt.Println("    -sql,   --enable-database        Enables support for the Postgres SQL database persistance. (default: disabled)")
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
	fmt.Println()
	fmt.Println("Example usage:")
	fmt.Println("    ", exe, "--port 8888")
}

const leftColumnWidth = 8

func PrettyPrintConfig(config Config) {
	width := 2 + MaxOf(
		len(config.Server.Address),
		LengthOfInt(int(config.Server.Port)),
		LengthOfBool(config.Server.EnableSsl),
		LengthOfBool(config.Server.EnableSubs),
		LengthOfBool(config.Logging.EnableColors),
		len(config.Server.Domain),
		len("VALUE"),
		len("-"),
	)

	format := &strings.Builder{}
	format.WriteString("+")
	WriteNTimes(format, "-", leftColumnWidth+1+width)
	format.WriteString("+\n|")
	CenterPad(format, "KEY", leftColumnWidth)
	format.WriteRune('|')
	CenterPad(format, "VALUE", width)
	format.WriteString("|\n+")
	WriteNTimes(format, "-", leftColumnWidth+1+width)
	format.WriteString("+\n")

	RowFormatKeyValue(width, format, "ip", config.Server.Address)
	RowFormatKeyValue(width, format, "port", config.Server.Port)
	RowFormatKeyValue(width, format, "domain", config.Server.Domain)
	RowFormatKeyValue(width, format, "ssl", config.Server.EnableSsl)
	RowFormatKeyValue(width, format, "subs", config.Server.EnableSubs)
	RowFormatKeyValue(width, format, "color", config.Logging.EnableColors)

	format.WriteString("+")
	WriteNTimes(format, "-", leftColumnWidth+1+width)
	format.WriteString("+\n")
	fmt.Println(format.String())
}

func RowFormatKeyValue(width int, builder *strings.Builder, key string, value any) {
	builder.WriteRune('|')
	LeftPad(builder, key, leftColumnWidth)
	builder.WriteRune('|')
	val := AnyToString(value)
	RightPad(builder, val, width)
	builder.WriteString("|\n")
}

func AnyToString(anything any) string {
	if anything == "" {
		return "-"
	}

	return fmt.Sprintf("%v", anything)
}

func CenterPad(builder *strings.Builder, text string, length int) {
	rem := length - len(text)
	half := rem / 2
	for range half {
		builder.WriteString(" ")
	}

	builder.WriteString(text)
	for i := half; i < rem; i++ {
		builder.WriteString(" ")
	}
}

func LeftPad(builder *strings.Builder, text string, length int) {
	rem := length - len(text) - 1

	for range rem {
		builder.WriteString(" ")
	}

	builder.WriteString(text)
	builder.WriteString(" ")
}

func RightPad(builder *strings.Builder, text string, length int) {
	rem := length - len(text) - 1

	builder.WriteString(" ")
	builder.WriteString(text)

	for range rem {
		builder.WriteString(" ")
	}
}

func WriteNTimes(builder *strings.Builder, character string, times int) {
	for range times {
		builder.WriteString(character)
	}
}

func LengthOfInt(n int) int {
	return len(strconv.Itoa(n))
}

func LengthOfBool(b bool) int {
	if b {
		return 4
	} else {
		return 5
	}
}

func MaxOf(values ...int) int {
	maxValue := 0
	for _, val := range values {
		if val > maxValue {
			maxValue = val
		}
	}
	return maxValue
}
