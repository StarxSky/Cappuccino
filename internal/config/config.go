package config 

type Config struct { 
	Interval int 
	Color int 
	AvgSeconds int 
	ShowCores bool 
	MaxCount int 
}

func DefaultConfig() *Config { 
	return &Config{
		Interval: 1,
		Color: 2,
		AvgSeconds: 30,
		ShowCores: false,
		MaxCount: 0,
	}
}