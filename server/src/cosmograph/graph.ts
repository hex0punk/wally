import { Cosmograph, CosmographSearch } from "@cosmograph/cosmograph";
import { BaseConfig } from "./config";
import { Node, Link } from "./types";

export class WallyGraph {
  data: any;
  nodes: Node[] = [];
  links: Link[] = [];
  clickedNodes: string[] = [];
  clickedNodeId: string = "";
  config: any;
  searchConfig: any;
  cosmograph: Cosmograph<Node, Link>;
  cosmoSearch: CosmographSearch<Node, Link>;

  constructor(data: any) {
    this.data = data;
    this.setConfig();
    this.setNodes();

    const cosmographContainer = document.getElementById("cosmograph")!;
    const searchContainer = document.getElementById("cosmosearch");

    this.cosmograph = new Cosmograph<Node, Link>(
      cosmographContainer as HTMLDivElement,
    );
    this.cosmoSearch = new CosmographSearch<Node, Link>(
      this.cosmograph,
      searchContainer as HTMLDivElement,
    );
  }

  private setConfig() {
    this.config = BaseConfig;
    this.config.nodeColor = (n: any) => this.getClickedNodesColor(n);
    this.config.linkColor = (l: any) => this.getLinkColor(l);
    this.config.nodeLabelAccessor = (n: any) => this.getLabel(n);
    this.config.onClick = (node: any, i: any) => this.onNodeClick(node, i);

    this.searchConfig = {
      maxVisibleItems: 5,
      activeAccessorIndex: 0,
      events: {
        onSelect: (node: { id: any }) => {
          console.log("Selected Node: ", node.id);
        },
      },
    };
  }

  private setNodes() {
    try {
      this.parseData();
    } catch (error) {
      console.error("Error loading Wally data:", error);
    }
  }

  private parseData() {
    this.data.forEach((finding: any) => {
      finding.Paths.forEach((paths: string[]) => {
        let prev = "";
        paths.forEach((path, i) => {
          if (i === 0) {
            prev = path;
            this.addNodeIfNotExist(path, "purple", "");
          } else {
            if (i == paths.length - 1) {
              this.addNodeIfNotExist(path, "#984040", finding.MatchId);
            } else {
              this.addNodeIfNotExist(path, "#4287f5", "");
            }
            this.addEdgeIfNotExist(prev, path);
            prev = path;
          }
        });
      });
    });
  }

  private nodeExists(nodeId: string): boolean {
    return this.nodes.some((node) => node.id === nodeId);
  }

  private edgeExists(source: string, target: string): boolean {
    return this.links.some(
      (link) => link.source === source && link.target === target,
    );
  }

  private addNodeIfNotExist(nodeId: string, color: string, findingId: string) {
    if (!this.nodeExists(nodeId)) {
      let label = this.extractFuncFromId(nodeId);
      label = label != null ? label : "";
      this.nodes.push({
        id: nodeId,
        label: label,
        color: color,
        green: "green",
        finding: findingId,
      });
    } else {
      let node = this.nodes.find((node) => node.id === nodeId);
      if (node != null) {
        if (color == "#4287f5" && node.color == "purple") {
          node.color = "#FFCE85";
        }

        if (node.color == "#4287f5" && color == "purple") {
          node.color = "#FFCE85";
        }
      }
    }
  }

  private addEdgeIfNotExist(source: string, target: string) {
    if (!this.edgeExists(source, target)) {
      this.links.push({ source, target, color: "#8C8C8C" });
    }
  }

  findAllPrecedingNodes(nodeId: string): any {
    let visited = new Set(); // To keep track of visited nodes
    let stack = [nodeId]; // Start with the target node

    while (stack.length > 0) {
      let current = stack.pop();

      // Add the current node to the visited set
      visited.add(current);

      // Find all links where the current node is a target
      let incomingLinks = this.links.filter(
        (link: { target: string | undefined }) => link.target === current,
      );
      incomingLinks.forEach((link: { source: string }) => {
        // Add the source node of each link to the stack
        if (!visited.has(link.source)) {
          stack.push(link.source);
        }
      });
    }

    visited.delete(nodeId); // Remove the initial node from the result
    return Array.from(visited); // Convert the Set of visited nodes to an Array and return
  }

  findLinksByNodeId(nodeId: string): Link[] {
    return this.links.filter(
      (link: { source: string; target: string }) =>
        link.source === nodeId || link.target === nodeId,
    );
  }

  onNodeClick(node: { id: string; finding: string } | undefined, i: any) {
    if (node == undefined) {
      this.clickedNodes = [];
      this.clickedNodeId = "";
      detailsOff();

      this.cosmoSearch.clearInput();
      this.cosmograph.unselectNodes();
    } else {
      let conn = this.findAllPrecedingNodes(node.id);
      this.clickedNodes = conn;
      this.clickedNodes.push(node.id);
      this.clickedNodeId = node.id;

      if (node.finding != "") {
        let finding = this.getFinding(node.finding);
        setLeftSide(finding);
      }
    }

    this.config.nodeColor = (n: Node) => this.getClickedNodesColor(n);
    this.config.linkColor = (l: Link) => this.getLinkColor(l);
    this.cosmograph.setConfig(this.config);
  }

  getClickedNodesColor(node: Node) {
    const defaultColor = node.color;

    if (this.clickedNodes.includes(node.id)) {
      return node.color;
    } else {
      if (this.clickedNodes.length > 0) {
        return [0, 0, 0, 0];
      } else {
        if (node.color == "purple" || node.color == "#984040") {
          return node.color;
        }
        return defaultColor;
      }
    }
  }

  getClickedNodeColor(node: Node) {
    const defaultColor = node.color;
    const clickedColor = "green";

    if (this.clickedNodeId == node.id) {
      return clickedColor;
    } else {
      return defaultColor;
    }
  }

  setupGraph() {
    this.cosmograph.setConfig(BaseConfig);
    this.cosmoSearch.setConfig(this.searchConfig);
    this.cosmograph.setData(this.nodes, this.links);
  }

  extractFuncFromId(nodeId: string): string | null {
    const match = nodeId.match(/\[(.*?)\]/);
    return match ? match[1] : null;
  }

  getLinkColor(link: Link) {
    let nt = this.clickedNodes.find((n) => link.target === n);
    if (nt != undefined && nt != null) {
      return "green";
    }
    if (this.clickedNodes.length > 0) {
      return [0, 0, 0, 0];
    }
    return link.color;
  }

  getLabel(node: Node) {
    if (this.clickedNodes.includes(node.id)) {
      return node.id;
    } else {
      if (this.clickedNodes.length > 0) {
        return "";
      } else {
        return node.label;
      }
    }
  }

  getFinding(findingId: string): any {
    let res: any;
    this.data.forEach((finding: any) => {
      if (findingId == finding.MatchId) {
        res = finding;
      }
    });
    return res;
  }
}

// Containers in UI
const details = document.getElementById("details");

export function detailsOn() {
  if (details != null && details.classList.contains("invisible")) {
    details.classList.remove("invisible");
  }
}

export function detailsOff() {
  if (details != null && !details.classList.contains("invisible")) {
    details.classList.add("invisible");
  }
}

export function setLeftSide(finding: any) {
  detailsOn();
  document.getElementById("pkg")!.textContent = finding.Indicator.Package;
  document.getElementById("func")!.textContent = finding.Indicator.Function;
  document.getElementById("params")!.textContent = JSON.stringify(
    finding.Indicator.Params,
  );
  document.getElementById("enclosedBy")!.textContent = finding.EnclosedBy;
  document.getElementById("pos")!.textContent = finding.Pos;
  document.getElementById("pathNum")!.textContent = finding.Paths.length;
}
