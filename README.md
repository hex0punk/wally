# Wally

![](Wally3.gif)

Static analysis tool for detecting and mapping RPC and HTTP routes in Go code.

## Why is it called wally?

Because [Wally](https://monkeyisland.fandom.com/wiki/Wally_B._Feed) is a catographer, I like Monkey Island, and I wanted it to be called that :).

## Why not just grep instead?

So you are analyzing a Go-based application and you need to find all HTTP and RPC routes. You can run grep or ripgrep to find specific patterns that'd point you to routes in the code but 

1. You'd need to parse through a lot of unnecessary strings
2. You may end up with functions that are similar to those you are targeting but have nothing to do with HTTP or RPC.
3. Grep won't solve constant values that indicate methods and route paths.

## What can Wally do that grep can't?

Wally currently supports the following features:

- Discover HTTP client calls and route listeners in your code by looking at each function name, signature, and package to make sure it finds the functions that you actually care about.
- Wally solves the value of compile-time constant values that may be used in the functions of interest. Wally does a pretty good job at finding constants and global variables and resolving their values for you so you don't have to chase those manually in code.
- Wally will report the enclosing function where the function of interest is called.
- Wally will also give you all possible call paths to your functions of interest. This can be useful when analyzing monorepos where service A calls service B via a client function declared in service B's packages. This feature requires that the target code base is buildable.
- Wally will output a nice PNG graph of the call stacks for the different routes it finds.

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

Wally should work even if you are not able to build the project you want to run it against. However, if you can build the project without any issues, you can run wally using the `--ssa` flag, at which point Wally will be able to do the following:

- Solve the enclosing function more effectively using [SSA](https://pkg.go.dev/golang.org/x/tools/go/ssa).
- Output all possible call paths to the functions where the routes are defined and/or called.

When using the `--ssa` flag you can expect output like this:

```shell
===========MATCH===============
Package:  net/http
Function:  Handle
Params:
	pattern: "/v1/client/metadata"
Enclosed by:  agent.registerHandlers
Position /Users/hex0punk/Tests/nomad/command/agent/http.go:444
Possible Paths: 6
	Path 1:
		n105973:(*github.com/hashicorp/nomad/command/agent.Command).Run --->
		n24048:(*github.com/hashicorp/nomad/command/agent.Command).setupAgent --->
		n24050:github.com/hashicorp/nomad/command/agent.NewHTTPServers --->
		n47976:(*github.com/hashicorp/nomad/command/agent.HTTPServer).registerHandlers --->
	Path 2:
		n104203:github.com/hashicorp/nomad/command/agent.NewTestAgent --->
		n92695:(*github.com/hashicorp/nomad/command/agent.TestAgent).Start --->
		n32861:(*github.com/hashicorp/nomad/command/agent.TestAgent).start --->
		n24050:github.com/hashicorp/nomad/command/agent.NewHTTPServers --->
		n47976:(*github.com/hashicorp/nomad/command/agent.HTTPServer).registerHandlers --->
	Path 3:
		n105973:(*github.com/hashicorp/nomad/command/agent.Command).Run --->
		n117415:(*github.com/hashicorp/nomad/command/agent.Command).handleSignals --->
		n79534:(*github.com/hashicorp/nomad/command/agent.Command).handleReload --->
		n79544:(*github.com/hashicorp/nomad/command/agent.Command).reloadHTTPServer --->
		n24050:github.com/hashicorp/nomad/command/agent.NewHTTPServers --->
		n47976:(*github.com/hashicorp/nomad/command/agent.HTTPServer).registerHandlers --->
```

### Filtering call path analysis

**NOTE: This is very important if you want to use the `ssa` call mapper feature described above**. When running wally in ssa mode against large code bases, you'd want to tell it to limit it's call path mapping work to only packages in the `module` of the code base you are running it against. For instance, going back to the nomad example, you'd want to run wally like so:

```shell
$ wally map -p ./... --ssa -vvv -f "github.com/hashicorp/nomad/"
```
Where `-f` defines a filter for the call stack search function. If you don't do this, wally may end up getting stuck in some loop as it encounters recursive calls or very lengthy paths in scary dependency forests. 

If using `-f` is not enough and you are seeing wally taking a very long time in the "solving call paths" step, wally may have encountered some sort or recursive call. In that case, you can use `-l` and an integer to limit the number of recursive calls wally makes when mapping call paths. This will limit the paths you see in the output, but using a high enough number should return helpful paths still. Experiment with `-l`, `-f`, or both to get the results you need or expect.

### PNG and XDOT Graph output 

When using the `--ssa` flag, you can also use `-g` or `--graph` to indicate a path for a PNG or XDOT containing a graphviz base graph of the call stacks. For example, running:

```shell
$ wally map -p ./... --ssa -vvv -f "github.com/hashicorp/nomad/" -g ./mygraph.png
```
From _nomad/command/agent_ will output this graph:

![](graphsample.png)

Specifying a filename with a `.xdot` extention will create an [xdot](https://graphviz.org/docs/outputs/canon/#xdot) file instead.

## Guesser mode

In the future, Wally will be able to make educated guesses for potential HTTP or RPC routes with no additional indicators. For now, you can define indicators with a wild flag package (`"*"`) if you are not able (or don't want) to tell Wally which package each function may be coming from.

## The power of Wally

At its core Wally is basically a function mapper. You can define functions in configuration files that have nothing to do with HTTP or RPC routes to obtain the same information that is described here.

## Logging

You can add logging statements as needed during development in any function with a `Navigator` receiver like this: `n.Logger.Debug("your message", "a key", "a value")`.

## I am seeing duplicate call stack paths in ssa mode

At the moment, wally will often give you duplicate stack paths, where you'd notice a path of, say, A->B->C is repeated a couple of times or more. Based on my testing and debugging this is a drawback of the [`cha`](https://pkg.go.dev/golang.org/x/tools@v0.16.1/go/callgraph/cha) algorithm from Go's `callgraph` package, which  wally uses for the call stack path functionality. I am experimenting with other available algorithms in `go/callgraph/` to determine what the best option to minimize such issues (while getting accurate call stacks) could be and will update wally's code accordingly. In the case that we stick to the `cha` algorithm, I will write code to filter duplicates. 

_Update:_ You should not be seeing any duplicates as I added a function to remove those. However, this adds more work to wally, so part of the future work still includes testing other callgraph algorithms thoroughly.

## Contributing

Feel free to open issues and send PRs.



