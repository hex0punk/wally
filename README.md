# Wally

![](Wally3.gif)

Static analysis tool for detecting and mapping RPC and HTTP routes.

## Why is it called wally?

Because [Wally](https://monkeyisland.fandom.com/wiki/Wally_B._Feed) is a catographer, I like Monkey Island, and I wanted it to be called that :).

## How can I play with it?

A good test project to run it against is [nomad](https://github.com/hashicorp/nomad) because it has a lot of routes set up and called all over the place. I suggest the following:

1. Clone this project
2. In a separate directory, clone [nomad](https://github.com/hashicorp/nomad)
3. Build this project by running `go build`
4. Navigate to the root of the directory where you cloned nomad (`path/to/nomad`)
5. Run the following to run `wally` against nomad

```shell
$ <path/to/wally/wally> map ./... -vvv
``` 

### Logging

You can add logging statements as needed during development in any function with a `Navigator` receiver like this: `n.Logger.Debug("your message", "a key", "a value")`.

### Experiments

Right now there is only one route indicator setup in analysis.go. You can add more as needed to test how `wally` behaves, then write code if you do not see the expected results. Just add another `RouteIndicator` to the `InitIndicators` function that includes the type of function you want `wally` to detect for you.

