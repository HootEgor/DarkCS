package config

import (
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
	"log"
	"sync"
)

type Config struct {
	Env      string `yaml:"env" env-default:"local"`
	Telegram struct {
		ApiKey  string `yaml:"api_key" env-default:""`
		AdminId int64  `yaml:"admin_id" env-default:"0"`
		BotName string `yaml:"bot_name" env-default:"DarkCSBot"`
		Enabled bool   `yaml:"enabled" env-default:"false"`
	} `yaml:"telegram"`
	OpenAI struct {
		ApiKey       string `yaml:"api_key" env-default:""`
		OverseerID   string `yaml:"overseer_id" env-default:""`
		ConsultantID string `yaml:"consultant_id" env-default:""`
		CalculatorID string `yaml:"calculator_id" env-default:""`
		DevPrefix    string `yaml:"dev_prefix" env-default:""`
	} `yaml:"openai"`
	Username string `yaml:"username" env-default:""`
	ImgPath  string `yaml:"img_path" env-default:""`
	Mongo    struct {
		Enabled     bool   `yaml:"enabled" env-default:"false"`
		Host        string `yaml:"host" env-default:"127.0.0.1"`
		Port        string `yaml:"port" env-default:"27017"`
		User        string `yaml:"user" env-default:"admin"`
		Password    string `yaml:"password" env-default:"pass"`
		Database    string `yaml:"database" env-default:""`
		SaveUrl     string `yaml:"save_url" env-default:""`
		ExpiredDays int    `yaml:"expired_days" env-default:"7"`
	} `yaml:"mongo"`
	ProdService struct {
		Login    string `yaml:"login" env-default:""`
		Password string `yaml:"password" env-default:""`
		BaseURL  string `yaml:"base_url" env-default:""`
	} `yaml:"prod-service"`
	Listen struct {
		BindIP string `yaml:"bind_ip" env-default:"127.0.0.1"`
		Port   string `yaml:"port" env-default:"9100"`
		ApiKey string `yaml:"key" env-default:""`
	} `yaml:"listen"`
}

var instance *Config
var once sync.Once

func MustLoad(path string) *Config {
	var err error
	once.Do(func() {
		instance = &Config{}
		if err = cleanenv.ReadConfig(path, instance); err != nil {
			desc, _ := cleanenv.GetDescription(instance, nil)
			err = fmt.Errorf("%s; %s", err, desc)
			instance = nil
			log.Fatal(err)
		}
	})
	return instance
}
