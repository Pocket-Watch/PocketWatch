package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ServerConfig struct {
	Address    string `json:"address"`
	Port       uint16 `json:"port"`
	Domain     string `json:"domain"`
	EnableSsl  bool   `json:"enable_ssl"`
	EnableSubs bool   `json:"enable_subs"`
}

type LoggingConfig struct {
	Enabled      bool   `json:"enabled"`
	EnableColors bool   `json:"enable_colors"`
	LogLevel     uint16 `json:"log_level"`
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
		Address:    "localhost",
		Port:       1234,
		Domain:     "example.com",
		EnableSsl:  false,
		EnableSubs: false,
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

func ApplyInputFlags(config *Config, flags InputFlags) {
	if flags.ServerAddress != "" {
		config.Server.Address = flags.ServerAddress
	}

	if flags.ServerPort != 0 {
		config.Server.Port = flags.ServerPort
	}

	if flags.ServerDomain != "" {
		config.Server.Domain = flags.ServerDomain
	}

	if flags.EnableSsl {
		config.Server.EnableSsl = true
	}

	if flags.EnableSubs {
		config.Server.EnableSubs = true
	}

	if flags.DisableColor {
		config.Logging.EnableColors = false
	}

	if flags.EnableSql {
		config.Database.Enabled = true
	}
}

type InputFlags struct {
	ConfigPath     string
	GenerateConfig bool
	ServerAddress  string
	ServerDomain   string
	ServerPort     uint16
	EnableSql      bool
	EnableSsl      bool
	EnableSubs     bool
	DisableColor   bool
	ShowHelp       bool
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

			flags.ServerPort = uint16(port)

		case "-dc", "--disable-colors":
			flags.DisableColor = true

		case "-es", "--enable-subs":
			flags.EnableSubs = false

		case "-sql", "--enable-sql":
			flags.EnableSql = true

		case "-ssl", "--enable-ssl":
			flags.EnableSsl = true

		case "-h", "--help":
			flags.ShowHelp = true

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
	fmt.Println("    -h,   --help                   Display this help message.")
	fmt.Println("    -cp,  --config-path [path]     Loads config from the provided path.")
	fmt.Println("    -gc,  --generate-config        Generates default config. Can also be use in combination with --config-path to specify output path. (default: ./config.json)")
	fmt.Println("    -ip,  --address [10.0.0.1]     Binds server to an address. (default: localhost)")
	fmt.Println("    -p,   --port [443]             Set address port to bind. (values between '0-65535') (default: 1234)")
	fmt.Println("    -dc,  --disable-color          Disables colored logging. (default: enabled)")
	fmt.Println("    -es,  --enable-subs            Enables support for subtitle search. (default: disabled)")
	fmt.Println("    -d,   --domain [example.com]   Domain, if any, that the server is hosted on. Serves as a hint to associate URLs with local env.")
	fmt.Println("    -sql, --enable-sql             Enables support for the Postgres SQL database persistance. (default: disabled)")
	fmt.Println("    -ssl, --enable-ssl             Enables SSL. Secrets are read from:")
	fmt.Println("                                     - CERTIFICATE: ./secret/certificate.pem")
	fmt.Println("                                     - PRIVATE KEY: ./secret/privatekey.pem")
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
