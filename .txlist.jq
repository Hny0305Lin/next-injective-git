def act: [.logs[0].events[]? | select(.type=="wasm") | .attributes[]? | select(.key=="action") | .value] | join(",");
.tx_responses[] | (.txhash[:10]) + "  " + (.timestamp[:16]) + "  " + (if (.logs|length) > 0 then (act | if . == "" then (.tx.body.messages[0]["@type"] | split(".") | last) else . end) else (.tx.body.messages[0]["@type"] | split(".") | last) end)
