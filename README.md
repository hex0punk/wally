# Wally

![](Wally3.gif)

Static analysis tool for detecting and mapping RPC and HTTP routes in Go code.

## Why is it called wally?

Because [Wally](https://monkeyisland.fandom.com/wiki/Wally_B._Feed) is a catographer, I like Monkey Island, and I wanted it to be called that :).

## Wally configurations

Wally needs a bit of hand-holding. Though it can also do a pretty good job at guessing paths, it helps a lot if you tell it the packages and functions to look for, along with the parameters that you are hoping to discover and map. So, to help Wally do the job you can specify a configuration file in YAML that defines a set of indicators. 

Wally runs a number of `indicators` which are basically clues as to whether a function in code may be related to a gRPC or HTTP route. However, sometimes a code base may have custom methods for setting up HTTP routes or for calling HTTP and RPC services. For instance, when reviewing Nomad, you can give Wally the following configuration file with Nomad specific indicators:

```yaml
indicators:
  - package: "github.com/hashicorp/nomad/command/agent"
    type: ""
    function: "forward"
    indicatorType: 1
    params:
      - name: "method"
  - package: "github.com/hashicorp/nomad/nomad"
    type: ""
    function: "RPC"
    indicatorType: 1
    params:
      - name: "method"
  - package: "github.com/hashicorp/nomad/api"
    type: "s"
    function: "query"
    indicatorType: 1
    params:
      - name: "endpoint"
        pos: 0
```

Note that you can specify the parameter that you want Wally to attempt to solve the value to. If you don't know the name of the parameter (per the function signature), you can give it the position in the signature. You can then use the `--config` or `-c` flag along with the path to the configuration file.



## How can I play with it?

A good test project to run it against is [nomad](https://github.com/hashicorp/nomad) because it has a lot of routes set up and called all over the place. I suggest the following:

1. Clone this project
2. In a separate directory, clone [nomad](https://github.com/hashicorp/nomad)
3. Build this project by running `go build`
4. Navigate to the root of the directory where you cloned nomad (`path/to/nomad`)
5. Create a configuration file named `.wally.yaml` with the content shown in the previous section of this README, and save it to the root of the nomad directory.
6. Run the following command from the nomad root:

```shell
$ <path/to/wally/wally> map -p ./... -vvv
```

## Wally's fanciest features

Wally should work even if you are not able to build the project you want to run it against. However, if you are able to build the project without any issues, you can run wally using the `--ssa` flag, at which point Wally will be able to do the following:



### Logging

You can add logging statements as needed during development in any function with a `Navigator` receiver like this: `n.Logger.Debug("your message", "a key", "a value")`.

### Experiments

Right now there is only one route indicator setup in analysis.go. You can add more as needed to test how `wally` behaves, then write code if you do not see the expected results. Just add another `RouteIndicator` to the `InitIndicators` function that includes the type of function you want `wally` to detect for you.

