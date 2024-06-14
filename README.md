<div align="center">
	<h1><img alt="Wally" src="Wally3.gif" height="250" /><br />
		wally the cartographer
	</h1>
</div>

Wally is a static analysis tool for mapping function paths in code. It can be used for:

- HTTP and gRPC route detection
- Attack surface mapping. 
- Automating the initial stages of threat modelling by mapping RPC and HTTP routes in Go code. 
- Planning fuzzing efforts by examining the fault tolenrace of call paths in code.

## UI Demo

https://github.com/hex0punk/wally/assets/1915998/1965f765-5437-4486-8c62-c125455b1f01

_Read about this graph and how to explore it in the [Exploring the graph with wally server](#Exploring-the-graph-with-wally-server) section_

## The basics

### Why is it called Wally?

Because [Wally](https://monkeyisland.fandom.com/wiki/Wally_B._Feed) is a cartographer, I like Monkey Island, and I wanted it to be called that :).

### Why not just grep instead?

So you are analyzing a Go-based application and you need to find all HTTP and RPC routes. You can run grep or ripgrep to find specific patterns that'd point you to routes in the code but:

1. You'd need to parse through a lot of unnecessary strings.
2. You may end up with functions that are similar to those you are targeting but have nothing to do with HTTP or RPC.
3. Grep won't solve constant values that indicate methods and route paths.

### What can Wally do that grep can't?

Wally currently supports the following features:

- Discover HTTP client calls and route listeners in your code by looking at each function name, signature, and package to make sure it finds the functions that you actually care about.
- Wally solves the value of compile-time constant values that may be used in the functions of interest. Wally does a pretty good job at finding constants and global variables and resolving their values for you so you don't have to chase those manually in code.
- Determine possible paths to a target function and examine the fault tolerance of such path. This is particularly useful when determining fuzzing targets or evaluating of panics discovered during fuzzing efforts.
- Wally will report the enclosing function where the function of interest is called.
- Wally will also give you all possible call paths to your functions of interest. This can be useful when analyzing monorepos where service A calls service B via a client function declared in service B's packages. This feature requires that the target code base is buildable.
- Wally will output a nice PNG graph of the call stacks for the different routes it finds.
- Determine which callpaths in code are tolerant to panics or application crashes due to bugs like nil dereferences

### Use case example

You are conducting an analysis of a monorepo containing multiple microservices. Often, these sorts of projects rely heavily on gRPC, which generates code for setting up gRPC routes via functions that call [`Invoke`](https://pkg.go.dev/google.golang.org/grpc#Invoke). Other services can then use these functions to call each other. 

One of the built-in indicators in `wally` will allow it to find functions that call `Invoke` for gRPC routes, so you can get a nice list of all gRPC method calls for all your microservices. Further, with `--ssa` you can also map the chains of methods gRPC calls necessary to reach any given gRPC route. With `wally`, you can then answer:

- Can users reach service `Y` hosted internally via service `A` hosted externally?
- Which service would I have to initialize a call to send user input to service `X`?
- What functions are there between service `A` and service `Y` that might sanitize or modify the input set to service `A`?

## Wally configurations

Wally needs a bit of hand-holding. Though it can also do a pretty good job at guessing paths, it helps a lot if you tell it the packages and functions to look for, along with the parameters that you are hoping to discover and map. So, to help Wally do the job, you can specify a configuration file in YAML that defines a set of indicators.

Wally runs a number of `indicators` which are basically clues as to whether a function in code may be related to a gRPC or HTTP route. By default, `wally` has a number of built-in `indicators` which check for common ways to set up and call HTTP and RPC methods using standard and popular libraries. However, sometimes a codebase may have custom methods for setting up HTTP routes or for calling HTTP and RPC services. For instance, when reviewing Nomad, you can give Wally the following configuration file with Nomad-specific indicators:

```yaml
indicators:
  - id: nomad-1
    package: "github.com/hashicorp/nomad/command/agent"
    type: ""
    function: "forward"
    indicatorType: 1
    receiverType: "ACL"    # optional 
    params:
      - name: "method"
  - id: nomad-2
    package: "github.com/hashicorp/nomad/nomad"
    type: ""
    function: "RPC"
    indicatorType: 1
    params:               # optional
      - name: "method"    # optional 
  - id: nomad-3
    package: "github.com/hashicorp/nomad/api"
    type: "s"
    function: "query"
    indicatorType: 1
    params:
        pos: 0           # optioncal
```

Note that you can specify the parameter that you want Wally to attempt to solve the value to. If you don't know the name of the parameter (per the function signature), you can give it the position in the signature. You can then use the `--config` or `-c` flag along with the path to the configuration file.

## Route Detection

A good test project to run it against is [nomad](https://github.com/hashicorp/nomad) because it has a lot of routes set up and called all over the place. I suggest the following:

1. Clone this project.
2. In a separate directory, clone [nomad](https://github.com/hashicorp/nomad).
3. Build this project by running `go build`.
4. Navigate to the root of the directory where you cloned nomad (`path/to/nomad`).
5. Create a configuration file named `.wally.yaml` with the content shown in the previous section of this README, and save it to the root of the nomad directory.
6. Run the following command from the nomad root:

```shell
$ <path/to/wally/wally> map -p ./... -vvv
```

## Running Wally with Docker

Wally can be easily run using Docker. Follow these steps:

1. Clone this project.
2. In a separate directory, clone [nomad](https://github.com/hashicorp/nomad).
3. Build the Docker Image:

    ```bash
    docker build -t go-wally .
    ```

4. Run an interactive shell inside the Docker container

    ```bash
    docker run -it go-wally /bin/sh
    ```

5. Run Wally with Docker, specifying the necessary parameters, such as the project path, configuration file, etc.:

    ```bash
    docker run -w /<PROJECT>/ -v $(pwd):/<PROJECT> go-wally map /<PROJECT>/... -vvv
    ```

   Adjust the flags (-p, -vvv, etc.) as needed for your use case.

6. If you have a specific configuration file (e.g., .wally.yaml), you can mount it into the container:

    ```bash
    docker run -w </PROJECT> -v $(pwd):</PROJECT> -v </PATH/TO/.wally.yaml>:</PROJECT>/.wally.yaml go-wally map -c .wally.yaml -p ./... --max-paths 50 -vvv
    ```

   This will run Wally within a Docker container, analyzing your Go code for HTTP and RPC routes based on the specified indicators and configurations.

7. Optionally, if you encountered any issues during the Docker build, you can revisit the interactive shell inside the container for further debugging.

8. After running Wally, you can check the results and the generated PNG or XDOT graph output, as explained in the README.


## Callpath analysis

Wally should work even if you are not able to build the project you want to run it against. However, if you can build the project without any issues, you can run Wally using the `--ssa` flag, at which point Wally will be able to do the following:

- Solve the enclosing function more effectively using [SSA](https://pkg.go.dev/golang.org/x/tools/go/ssa).
- Output all possible call paths to the functions where the routes are defined and/or called.

When using the `--ssa` flag you can expect output like this:

```shell
===========MATCH===============
ID:  14554c2a-41ee-4634-831d-6fc49c70c80d
Indicator ID:  1
Package:  github.com/hashicorp/cronexpr
Function:  Parse
Params:
Enclosed by:  (*github.com/hashicorp/nomad/nomad/structs.PeriodicConfig).Validate
Position /Users/hex0punk/Tests/nomad/nomad/structs/structs.go:5638
Possible Paths: 1
	Path 1 (filter limited):
		[Validate] nomad/structs/structs.go:5614:26 --->

===========MATCH===============
ID:  6a876579-6b72-4501-af5b-5028c84a1c77
Indicator ID:  1
Package:  github.com/hashicorp/cronexpr
Function:  Parse
Params:
Enclosed by:  (*github.com/hashicorp/nomad/nomad/structs.PeriodicConfig).Validate
Position /Users/hex0punk/Tests/nomad/nomad/structs/structs.go:5644
Possible Paths: 1
	Path 1 (filter limited):
		[Validate] nomad/structs/structs.go:5614:26 --->

===========MATCH===============
ID:  eeaa94b1-28a8-41b8-a1e3-7a0d665a1e4d
Indicator ID:  1
Package:  github.com/hashicorp/cronexpr
Function:  Parse
Params:
Enclosed by:  github.com/hashicorp/nomad/nomad/structs.CronParseNext
Position /Users/hex0punk/Tests/nomad/nomad/structs/structs.go:5677
Possible Paths: 28
	Path 1 (RECOVERABLE):
		nomad.[Plan] nomad/job_endpoint.go:1949:57 --->
		structs.[Next] nomad/structs/structs.go:5693:24 --->
		[CronParseNext] (recoverable) nomad/structs/structs.go:5670:6 --->
	Path 2 (RECOVERABLE):
		nomad.[Plan] nomad/job_endpoint.go:1949:57 --->
		structs.[Next] nomad/structs/structs.go:5699:27 --->
		[CronParseNext] (recoverable) nomad/structs/structs.go:5670:6 --->
	Path 3 (node limited) (RECOVERABLE):
		nomad.[leaderLoop] nomad/leader.go:247:34 --->
		nomad.[establishLeadership] nomad/leader.go:412:33 --->
		nomad.[SetEnabled] nomad/periodic.go:167:3 --->
		nomad.[run] nomad/periodic.go:332:14 --->
		nomad.[dispatch] nomad/periodic.go:342:38 --->
		structs.[Next] nomad/structs/structs.go:5693:24 --->
		[CronParseNext] (recoverable) nomad/structs/structs.go:5670:6 --->
```

### Filtering call path analysis

> [!TIP]
> When running Wally in SSA mode against large codebases wally might run get lost in external libraries used by the target code. In most cases, you'd want to filter analysis to only the module you want to target. For instance, when using wally to find HTTP and gRPC routes in nomad, you'd want to type the command below. 

```shell
$ wally map -p ./... --ssa -vvv -f "github.com/hashicorp/nomad/" --max-paths 50
```

Where `-f` defines a filter for the call stack search function. If you don't do this, wally may end up getting stuck in some loop as it encounters recursive calls or very lengthy paths in scary dependency forests.

> [!IMPORTANT]
> If using `-f` is not enough, and you are seeing Wally taking a very long time in the "solving call paths" step, Wally may have encountered some sort of recursive call. In that case, you can use `--max-paths` and an integer to limit the number of recursive calls Wally makes when mapping call paths (50 tends to be a good number). This will limit the paths you see in the output, but using a high enough number should still return helpful paths. Experiment with `--max-paths`, `--max-funcs`, `-f`, or all three to get the results you need or expect.

### Analyzing individual paths

Rather than using a yaml configuration file, you can use `wally map search` for mapping paths to individual functions. For instance:

```bash
$ wally map search  -p ./... --func Parse --pkg github.com/hashicorp/cronexpr --max-funcs 7 --max-paths 50 -f github.com/hashicorp/ -vvv
```
The options above map to the following

- `-p ./...`: Target code is in the current directory
- `--func Parse`: We are interested only in the `Parse` function
- `--pkg github.com/hashicorp/cronexpr`: Of package `github.com/hashicorp/cronexpr`
- `--max-funcs 7`: We only want up to 7 functions per path
- `--max-paths 50`: Limit the paths to 50
- `-vvv`: Very, very verbose
- `-f github.com/hashicorp/`: This tells Wally that we are only interested in paths within packages that start with `github.com/hashicorp/`. This avoids getting paths that reach beyond the scope we are interested in. Otherwise, we'd get nodes in standard Go libraries, etc.

## Using Wally in Fuzzing Efforts to Determine Fault Tolerance of Call Paths

Wally can now tell you which paths to a target function will recover in case of a panic triggered by that target function. A detailed explanation can be found [here](https://hex0punk.com/posts/fault-tolerance-detection-with-wally/).

### Visualizing paths with wally

To make visualization of callpaths easier, wally can lunch a server on localhost when via a couple methods:

After an analysis by passing the `--server` flag to the `map` command. For instance:

```shell
$ wally map -p ./... -c .wally.yaml --ssa -f "github.com/hashicorp/nomad" --server
```

Or, using the `server` subcommand and passing a wally json  file:

```shell
 $ wally server -p ./nomad-wally.json -P 1984
```

Next, open a browser and head to the address in the output.

## Exploring the graph with wally server

Graphs are generated using the [cosmograph](https://cosmograph.app/) library. Each node represents a function call in code. The colors are not random. Each color has a a different purpose to help you make good use of the graph.

![](assets/finding-node.svg)
<span style="vertical-align: top;">Finding node. This is a node discovered via wally indicators. Every finding node is the end of a path</span>

![](assets/root-node.svg)
<span style="vertical-align: top;">This node is the root of a path to a finding node.</span>

![](assets/path-node.svg)
<span style="vertical-align: top;">Intermediate node between a root and a finding node.</span>

![](assets/dual-node.svg)
<span style="vertical-align: top;">This node servers both as the root node to a path and an intermediary node for one or more paths</span>

### Viewing paths

Clicking on any node will highlight all possible paths to that node. Click anywhere other than a node to exist the path selection view.

### Viewing findings

Clicking on any finding node will populate the section on the left with information about the finding.

### Searching nodes

Start typing on the search bar on the left to find a node by name. 

### PNG and XDOT Graph output

When using the `--ssa` flag, you can also use `-g` or `--graph` to indicate a path for a PNG or XDOT containing a Graphviz-based graph of the call stacks. For example, running:

```shell
$ wally map -p ./... --ssa -vvv -f "github.com/hashicorp/nomad/" -g ./mygraph.png
```

From _nomad/command/agent_ will output this graph:

![](graphsample.png)

Specifying a filename with a `.xdot` extension will create an [xdot](https://graphviz.org/docs/outputs/canon/#xdot) file instead.

## Advanced options

- You can specify which algorithm to use for the intial callgraph generation using `--callgraph-alg`. This is the algorithm used by the `golang.org/x/tools/` function. Options include `cha` (default), [`rta`](https://pkg.go.dev/golang.org/x/tools/go/callgraph/rta), and [`vta`](https://pkg.go.dev/golang.org/x/tools/go/callgraph/vta).
- By default, wally uses a breathd search first algorithm to map all paths. You can instead use depth first search using `--search-alg dfs`
- Whenever Wally sees it reaches a `main` function, it will stop going further back in the tree to avoid reporting inaccurate paths. If you wish, you can override this by using the `--continue-after-main` flag, allowing you to see some interesting but less likely paths.

## The power of Wally

At its core, Wally is, essentially, a function mapper. You can define functions in configuration files that have nothing to do with HTTP or RPC routes to obtain the same information that is described here.

## Logging

You can add logging statements as needed during development in any function with a `Navigator` receiver like this: `n.Logger.Debug("your message", "a key", "a value")`.

## Troubleshooting

At the moment, wally will often give you duplicate stack paths, where you'd notice a path of, say, A->B->C is repeated a couple of times or more. Based on my testing and debugging this is a drawback of the [`cha`](https://pkg.go.dev/golang.org/x/tools@v0.16.1/go/callgraph/cha) algorithm from Go's `callgraph` package, which wally uses for the call stack path functionality. I am experimenting with other available algorithms in `go/callgraph/` to determine what the best option to minimize such issues (while getting accurate call stacks) could be and will update wally's code accordingly. In the case that we stick to the `cha` algorithm, I will write code to filter duplicates.

### When running in SSA mode, I get findings with no enclosed functions reported

This is often caused by issues in the target code base. Make sure you are able to build the target codebase. You may want to run `go build` and fix any issues reported by the compiler. Then, run wally again against it.

### Wally appears to be stuck in loop

See the section on [Filtering call path analysis](#Filtering-call-path-analysis)

## Viewing help

Viewing the description of each command

### `map`

```Shell
$ wally map --help
```

#### `map search` (single function)

```Shell
$ wally map search --help
```

### `server`

```Shell
$ wally map --help
```

## Contributing

Feel free to open issues and send PRs. Please.
