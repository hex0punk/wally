package indicator

type IndicatorType int

const (
	SERVICE IndicatorType = iota
	CALLER
)

type Indicator struct {
	Package        string
	Type           string
	Function       string
	Params         []RouteParam
	RouteParamPos  int
	RouteParamName string
	IndicatorType  IndicatorType
}

type RouteParam struct {
	Name string
	Pos  int
}

func InitIndicators() []Indicator {
	return []Indicator{
		{
			Package:        "github.com/hashicorp/nomad/nomad",
			Type:           "",
			Function:       "forward",
			RouteParamName: "method",
			//RouteParamPos:  0,
		},
		{
			Package:        "net/http",
			Type:           "",
			Function:       "Handle",
			RouteParamName: "pattern",
			//RouteParamPos:  0,
		},
		{
			Package:       "github.com/hashicorp/nomad/command/agent",
			Type:          "",
			Function:      "RPC",
			RouteParamPos: 0,
		},
		{
			Package:        "*",
			Type:           "",
			Function:       "Register",
			RouteParamName: "pattern",
		},
	}
}
