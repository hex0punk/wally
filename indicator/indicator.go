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
	Package       string
	Type          string
	Function      string
	Params        []RouteParam
	IndicatorType IndicatorType
}

type RouteParam struct {
	Name string
	Pos  int
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
