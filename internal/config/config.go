package config

import (
	"encoding/json"
	"flag"
	"os"
)

// DBConfig represents database configuration
type DBConfig struct {
	DSN string `json:"dsn"`
}

// ServerConfig represents the server configuration structure
type ServerConfig struct {
	Address       string   `json:"address"`
	Restore       bool     `json:"restore"`
	StoreFile     string   `json:"store_file"`
	CryptoKey     string   `json:"crypto_key"`
	StoreInterval string   `json:"store_interval"`
	DB            DBConfig `json:"db"`
	TrustedSubnet string   `json:"trusted_subnet,omitempty"`
}

// AgentConfig represents the agent configuration structure
type AgentConfig struct {
	Address        string `json:"address"`
	ReportInterval string `json:"report_interval"`
	PollInterval   string `json:"poll_interval"`
	CryptoKey      string `json:"crypto_key"`
}

// LoadServerConfig loads server configuration from a JSON file
func LoadServerConfig(configPath string) (*ServerConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config ServerConfig
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadAgentConfig loads agent configuration from a JSON file
func LoadAgentConfig(configPath string) (*AgentConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config AgentConfig
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// GetConfigPath determines the configuration file path from flag or environment variable
func GetConfigPath(configFlagName string) string {
	var configPath string

	// Check if flag is defined before parsing
	if flag.Lookup(configFlagName) == nil {
		flag.StringVar(&configPath, configFlagName, "", "path to config file")
	} else {
		// If flag already exists, get its value
		configPath = *flag.String(configFlagName, "", "path to config file")
	}

	// Check environment variable
	envConfigPath := os.Getenv("CONFIG")
	if envConfigPath != "" {
		return envConfigPath
	}

	// Parse flags to check if config flag was provided
	if !flag.Parsed() {
		flag.Parse()
	}

	if configPath != "" {
		return configPath
	}

	return ""
}
