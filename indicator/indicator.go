package indicator

type IndicatorType int

const (
	Service IndicatorType = iota
	Caller
)

type ParamType int

const (
	HTTPMethod ParamType = iota
	Path
)

type Indicator struct {
	Package       string        `yaml:"package"`
	Type          string        `yaml:"type"`
	Function      string        `yaml:"function"`
	Params        []RouteParam  `yaml:"params"`
	IndicatorType IndicatorType `yaml:"indicatorType"`
}

type RouteParam struct {
	Name string `yaml:"name"`
	Pos  int    `yaml:"pos"`
}

func InitIndicators() []Indicator {
	return []Indicator{
		{
			Package:  "github.com/hashicorp/nomad/nomad",
			Type:     "",
			Function: "forward",
			Params: []RouteParam{
				{Name: "method"},
			},
			IndicatorType: Caller,
		},
		{
			Package:  "github.com/hashicorp/nomad/command/agent",
			Type:     "",
			Function: "RPC",
			Params: []RouteParam{
				{Pos: 0},
			},
			IndicatorType: Caller,
		},
		{
			Package:  "github.com/hashicorp/nomad/api",
			Type:     "",
			Function: "query",
			Params: []RouteParam{
				{Name: "endpoint"},
			},
			IndicatorType: Caller,
		},
		{
			Package:  "net/http",
			Type:     "",
			Function: "Handle",
			Params: []RouteParam{
				{Name: "pattern"},
			},
			IndicatorType: Service,
		},
		//{
		//	Package:        "*",
		//	Type:           "",
		//	Function:       "Register",
		//	RouteParamName: "pattern",
		//},
	}
}
