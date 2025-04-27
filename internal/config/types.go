package config

type Service struct {
	Runtime      string            `yaml:"runtime"`
	Image        string            `yaml:"image"`
	Architecture string            `yaml:"architecture"`
	CodePath     string            `yaml:"codePath"`
	Cmd          []string          `yaml:"cmd"`
	Entrypoint   []string          `yaml:"entrypoint"`
	Environment  map[string]string `yaml:"environment"`
}

type Route struct {
	Path    string `yaml:"path"`
	Service string `yaml:"service"`
	Method  string `yaml:"method"`
}

type APIGateway struct {
	Port   string  `yaml:"port"`
	Routes []Route `yaml:"routes"`
	Stage  string  `yaml:"stage"`
}

type Config struct {
	APIGateway APIGateway         `yaml:"apiGatewayPort"`
	Services   map[string]Service `yaml:"services"`
	Host       string             `yaml:"-"`
}
