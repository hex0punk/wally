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
	Id            string        `yaml:"id"`
	Package       string        `yaml:"package"`
	Type          string        `yaml:"type"`
	Function      string        `yaml:"function"`
	Params        []RouteParam  `yaml:"params"`
	IndicatorType IndicatorType `yaml:"indicatorType"`
	ReceiverType  string        `yaml:"receiverType"`
}

type RouteParam struct {
	Name string `yaml:"name"`
	Pos  int    `yaml:"pos"`
}

func InitIndicators(customIndicators []Indicator, skipDefault bool) []Indicator {
	indicators := []Indicator{}
	if !skipDefault {
		indicators = getStockIndicators()
	}

	if customIndicators != nil && len(customIndicators) > 0 {
		fmt.Println("Loading custom indicator")
		idStart := len(indicators)
		for i, ind := range customIndicators {
			indCpy := ind
			if indCpy.Id == "" {
				indCpy.Id = fmt.Sprintf("%d", idStart+i+1)
			}
			fmt.Println("Pkg: ", indCpy.Package)
			fmt.Println("Func: ", indCpy.Function)
			if indCpy.ReceiverType != "" {
				fmt.Println("Receiver Type: ", indCpy.ReceiverType)
			}
			fmt.Println()
			indicators = append(indicators, indCpy)
		}
	}
	return indicators
}

func getStockIndicators() []Indicator {
	return []Indicator{
		{
			Id:       "1",
			Package:  "net/http",
			Type:     "",
			Function: "Handle",
			Params: []RouteParam{
				{Name: "pattern"},
			},
			IndicatorType: Service,
		},
		{
			Id:       "2",
			Package:  "google.golang.org/grpc",
			Type:     "",
			Function: "Invoke",
			Params: []RouteParam{
				{Name: "method"},
			},
			IndicatorType: Service,
		},
	}
}
