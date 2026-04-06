package config

import "os"

type Config struct {
	Port     string
	DBPath   string
	ARRPath  string
	AuthPath string
}

func Load() *Config {
	port := os.Getenv("QBITCTRL_PORT")
	if port == "" { port = "9911" }
	db := os.Getenv("QBITCTRL_DB")
	if db == "" { db = "qbitctrl_servers.json" }
	arr := os.Getenv("QBITCTRL_ARR")
	if arr == "" { arr = "qbitctrl_arr.json" }
	authPath := os.Getenv("QBITCTRL_AUTH")
	if authPath == "" { authPath = "qbitctrl_auth.json" }
	return &Config{Port: port, DBPath: db, ARRPath: arr, AuthPath: authPath}
}
