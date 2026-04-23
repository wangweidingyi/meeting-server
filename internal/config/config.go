package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	UDP      UDPConfig      `json:"udp"`
	MQTT     MQTTConfig     `json:"mqtt"`
	HTTP     HTTPConfig     `json:"http"`
	Database DatabaseConfig `json:"database"`
	AI       AIConfig       `json:"ai"`
}

type UDPConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type MQTTConfig struct {
	Enabled    bool   `json:"enabled"`
	Embedded   bool   `json:"embedded"`
	ListenHost string `json:"listenHost"`
	ListenPort int    `json:"listenPort"`
	BrokerURL  string `json:"brokerUrl"`
	ClientID   string `json:"clientId"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

type HTTPConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type DatabaseConfig struct {
	URL string `json:"url"`
}

type AIConfig struct {
	STT ModelProviderConfig  `json:"stt"`
	LLM ModelProviderConfig  `json:"llm"`
	TTS SpeechProviderConfig `json:"tts"`
}

type ModelProviderConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
}

type SpeechProviderConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
}

func Default() Config {
	return Config{
		UDP: UDPConfig{
			Host: "127.0.0.1",
			Port: 6000,
		},
		MQTT: MQTTConfig{
			Enabled:    false,
			Embedded:   false,
			ListenHost: "127.0.0.1",
			ListenPort: 1883,
			ClientID:   "meeting-server",
		},
		HTTP: HTTPConfig{
			Host: "127.0.0.1",
			Port: 8090,
		},
		AI: AIConfig{
			STT: ModelProviderConfig{
				Provider: "stub",
			},
			LLM: ModelProviderConfig{
				Provider: "stub",
			},
			TTS: SpeechProviderConfig{
				Provider: "stub",
			},
		},
	}
}

func LoadFromEnv() (Config, error) {
	cfg := Default()

	if value := os.Getenv("MEETING_UDP_HOST"); value != "" {
		cfg.UDP.Host = value
	}

	if value := os.Getenv("MEETING_UDP_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse MEETING_UDP_PORT: %w", err)
		}
		cfg.UDP.Port = port
	}

	if value := os.Getenv("MEETING_HTTP_HOST"); value != "" {
		cfg.HTTP.Host = value
	}

	if value := os.Getenv("MEETING_HTTP_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse MEETING_HTTP_PORT: %w", err)
		}
		cfg.HTTP.Port = port
	}

	if value := os.Getenv("MEETING_DATABASE_URL"); value != "" {
		cfg.Database.URL = value
	}

	if value := os.Getenv("MEETING_MQTT_EMBEDDED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse MEETING_MQTT_EMBEDDED: %w", err)
		}
		cfg.MQTT.Embedded = enabled
		if enabled {
			cfg.MQTT.Enabled = true
		}
	}

	if value := os.Getenv("MEETING_MQTT_LISTEN_HOST"); value != "" {
		cfg.MQTT.ListenHost = value
	}

	if value := os.Getenv("MEETING_MQTT_LISTEN_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse MEETING_MQTT_LISTEN_PORT: %w", err)
		}
		cfg.MQTT.ListenPort = port
	}

	if value := os.Getenv("MEETING_MQTT_BROKER"); value != "" {
		cfg.MQTT.Enabled = true
		cfg.MQTT.BrokerURL = value
	}

	if value := os.Getenv("MEETING_MQTT_CLIENT_ID"); value != "" {
		cfg.MQTT.ClientID = value
	}

	if value := os.Getenv("MEETING_MQTT_USERNAME"); value != "" {
		cfg.MQTT.Username = value
	}

	if value := os.Getenv("MEETING_MQTT_PASSWORD"); value != "" {
		cfg.MQTT.Password = value
	}

	if value := os.Getenv("MEETING_STT_PROVIDER"); value != "" {
		cfg.AI.STT.Provider = value
	}
	if value := os.Getenv("MEETING_STT_BASE_URL"); value != "" {
		cfg.AI.STT.BaseURL = value
	}
	if value := os.Getenv("MEETING_STT_API_KEY"); value != "" {
		cfg.AI.STT.APIKey = value
	}
	if value := os.Getenv("MEETING_STT_MODEL"); value != "" {
		cfg.AI.STT.Model = value
	}

	if value := os.Getenv("MEETING_LLM_PROVIDER"); value != "" {
		cfg.AI.LLM.Provider = value
	}
	if value := os.Getenv("MEETING_LLM_BASE_URL"); value != "" {
		cfg.AI.LLM.BaseURL = value
	}
	if value := os.Getenv("MEETING_LLM_API_KEY"); value != "" {
		cfg.AI.LLM.APIKey = value
	}
	if value := os.Getenv("MEETING_LLM_MODEL"); value != "" {
		cfg.AI.LLM.Model = value
	}

	if value := os.Getenv("MEETING_TTS_PROVIDER"); value != "" {
		cfg.AI.TTS.Provider = value
	}
	if value := os.Getenv("MEETING_TTS_BASE_URL"); value != "" {
		cfg.AI.TTS.BaseURL = value
	}
	if value := os.Getenv("MEETING_TTS_API_KEY"); value != "" {
		cfg.AI.TTS.APIKey = value
	}
	if value := os.Getenv("MEETING_TTS_MODEL"); value != "" {
		cfg.AI.TTS.Model = value
	}
	if value := os.Getenv("MEETING_TTS_VOICE"); value != "" {
		cfg.AI.TTS.Voice = value
	}

	return cfg, nil
}

func (c Config) Summary() string {
	mqttState := "disabled"
	if c.MQTT.Enabled {
		mqttState = fmt.Sprintf(
			"enabled broker=%s client_id=%s",
			c.MQTT.BrokerURL,
			c.MQTT.ClientID,
		)
	}

	databaseState := "disabled"
	if c.Database.URL != "" {
		databaseState = "configured"
	}

	return fmt.Sprintf(
		"udp=%s:%d http=%s:%d db=%s mqtt=%s",
		c.UDP.Host,
		c.UDP.Port,
		c.HTTP.Host,
		c.HTTP.Port,
		databaseState,
		mqttState,
	)
}
