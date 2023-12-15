package indicator

import "fmt"

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

func InitIndicators(customIndicators []Indicator) []Indicator {
	indicators := getStockIndicators()
	if customIndicators != nil && len(customIndicators) > 0 {
		fmt.Println("Loading custom indicator")
		for _, ind := range customIndicators {
			indCpy := ind
			fmt.Println("Pkg: ", indCpy.Package)
			fmt.Println("Func: ", indCpy.Function)
			fmt.Println()
			indicators = append(indicators, indCpy)
		}
	}
	return indicators
}

func getStockIndicators() []Indicator {
	return []Indicator{
		{
			Package:  "net/http",
			Type:     "",
			Function: "Handle",
			Params: []RouteParam{
				{Name: "pattern"},
			},
			IndicatorType: Service,
		},
	}
}
