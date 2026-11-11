export function repositoryPath(owner: string, name: string, search = ""): string {
  const suffix = search.startsWith("?") ? search : search ? "?" + search : "";
  return "/" + encodeURIComponent(owner) + "/" + encodeURIComponent(name) + suffix;
}
