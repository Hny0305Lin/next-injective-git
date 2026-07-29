.workflow_runs[:3][] | [.name, .head_branch, .status, (.conclusion // "running"), .head_sha[:7]] | join(" / ")
