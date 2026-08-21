export type DemoStatus = "bootstrapped" | "active";

export interface DemoSuite {
  directory: string;
  chainId: 1439;
  status: DemoStatus;
}

export const demoSuite: DemoSuite = {
  directory: "0xf8844F90887731FFd607E1f59e39a3918F6eAb35",
  chainId: 1439,
  status: "active",
};

export function describeSuite(suite: DemoSuite): string {
  return `${suite.status} suite on chain ${suite.chainId}: ${suite.directory}`;
}
