export type Node = {
  id: string;
  color: string;
  green: string;
  label: string;
  finding: string;
  x?: number;
  y?: number;
};

export type Link = {
  source: string;
  target: string;
  color: string;
};
