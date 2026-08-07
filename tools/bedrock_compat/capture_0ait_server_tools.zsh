#!/bin/zsh

set -u

# Read the two already-authorized fixture keys interactively so they never live
# in the repository or in the capture document.
read -s AIT_LOW
printf '\n'
read -s AIT_HIGH
printf '\n'

DOC='/tmp/sub2api-0ait-aws-original-responses.md'
STATUS='/tmp/sub2api-0ait-aws-original-status.tsv'
WORK=$(cat /tmp/sub2api-0ait-capture-workdir)

append_headers() {
  awk 'tolower($0) ~ /^set-cookie:/ {next} {sub(/\r$/, ""); print}' "$1" >> "$DOC"
}

capture_server_tool() {
  local model="$1"
  local scenario="$2"
  local key="$3"
  local beta="$4"
  local payload="$5"
  local stream="$6"
  local safe_model headers body http_code curl_rc result_type
  local -a stream_arg beta_arg

  safe_model=$(printf '%s' "$model" | tr -c 'A-Za-z0-9._-' '_')
  headers="$WORK/server-${safe_model}-${scenario}.headers"
  body="$WORK/server-${safe_model}-${scenario}.body"
  stream_arg=()
  beta_arg=()
  if [[ "$stream" == true ]]; then
    stream_arg=(-N)
  fi
  if [[ -n "$beta" ]]; then
    beta_arg=(-H "anthropic-beta: $beta")
  fi

  http_code=$(curl --http1.1 --retry 2 --retry-all-errors --retry-delay 1 \
    -sS "${stream_arg[@]}" --max-time 180 \
    -D "$headers" -o "$body" -w '%{http_code}' \
    'https://api.0ait.com/v1/messages' \
    -H "Authorization: Bearer $key" \
    -H "x-api-key: $key" \
    -H 'anthropic-version: 2023-06-01' \
    -H 'content-type: application/json' \
    "${beta_arg[@]}" \
    --data-binary "$payload")
  curl_rc=$?

  {
    printf '## %s / %s\n\n' "$model" "$scenario"
    printf '**Request**\n\n````json\n'
    printf '%s' "$payload" | jq . 2>/dev/null || printf '%s\n' "$payload"
    printf '\n````\n\n'
    if [[ -n "$beta" ]]; then
      printf '**HTTP request beta header:** `%s`\n\n' "$beta"
    fi
    printf '**HTTP status:** `%s`\n\n' "$http_code"
    printf '**Response headers**\n\n````http\n'
  } >> "$DOC"
  append_headers "$headers"

  if [[ "$stream" == true ]]; then
    {
      printf '````\n\n**Raw SSE body**\n\n````text\n'
      command cat "$body"
      printf '\n````\n\n'
    } >> "$DOC"
    result_type='sse'
  else
    {
      printf '````\n\n**Raw JSON body**\n\n````json\n'
      jq . "$body" 2>/dev/null || command cat "$body"
      printf '\n````\n\n'
    } >> "$DOC"
    result_type=$(jq -r '.type // "unknown"' "$body" 2>/dev/null)
  fi

  printf '%s\t%s\t%s\t%s\t%s\n' \
    "$model" "$scenario" "$http_code" "$curl_rc" "$result_type" >> "$STATUS"
  printf 'captured %s %s http=%s rc=%s\n' \
    "$model" "$scenario" "$http_code" "$curl_rc"
}

models=(
  claude-opus-4-6
  claude-sonnet-4-6
  claude-opus-4-5-20251101
  claude-sonnet-4-5-20250929
  claude-haiku-4-5-20251001
  claude-opus-4-7
  claude-opus-4-8
  claude-opus-5
  claude-sonnet-5
)

for model in "${models[@]}"; do
  case "$model" in
    claude-opus-4-7|claude-opus-4-8|claude-opus-5|claude-sonnet-5)
      key="$AIT_HIGH"
      ;;
    *)
      key="$AIT_LOW"
      ;;
  esac

  # Anthropic server-side bash tool.
  bash_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the bash server tool to run the command printf server-tool-probe."}],tools:[{type:"bash_20250124",name:"bash"}],tool_choice:{type:"tool",name:"bash"}}')
  capture_server_tool "$model" 'bash-nonstream' "$key" '' "$bash_payload" false
  sleep 0.5
  capture_server_tool "$model" 'bash-stream' "$key" '' "$(printf '%s' "$bash_payload" | jq -c '. + {stream:true}')" true
  sleep 0.5

  # Anthropic server-side text editor tool.
  editor_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the text editor server tool to replace the word OLD with NEW in /tmp/probe.txt."}],tools:[{type:"text_editor_20250728",name:"str_replace_based_edit_tool"}],tool_choice:{type:"tool",name:"str_replace_based_edit_tool"}}')
  capture_server_tool "$model" 'text-editor-nonstream' "$key" '' "$editor_payload" false
  sleep 0.5
  capture_server_tool "$model" 'text-editor-stream' "$key" '' "$(printf '%s' "$editor_payload" | jq -c '. + {stream:true}')" true
  sleep 0.5

  # Memory is currently beta-gated; capture both the normal request and the
  # context-management beta variant so validation errors are documented too.
  memory_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the memory server tool to view /probe."}],tools:[{type:"memory_20250818",name:"memory"}],tool_choice:{type:"tool",name:"memory"}}')
  capture_server_tool "$model" 'memory-nonstream' "$key" '' "$memory_payload" false
  sleep 0.5
  capture_server_tool "$model" 'memory-context-beta-nonstream' "$key" 'context-management-2025-06-27' "$memory_payload" false
  sleep 0.5

  # Tool search is the one server tool that returns server_tool_use and
  # tool_search_tool_result blocks. Include deferred tools to force a result.
  regex_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:256,messages:[{role:"user",content:"Find a deferred tool that can look up the weather for Paris, then invoke it."}],tools:[{type:"tool_search_tool_regex_20251119",name:"tool_search_tool_regex"},{name:"get_weather",description:"Get current weather for a city",input_schema:{type:"object",properties:{city:{type:"string"}},required:["city"]},defer_loading:true},{name:"get_time",description:"Get local time for a city",input_schema:{type:"object",properties:{city:{type:"string"}},required:["city"]},defer_loading:true}],tool_choice:{type:"tool",name:"tool_search_tool_regex"}}')
  regex_beta='tool-search-tool-2025-10-19,tool-examples-2025-10-29'
  capture_server_tool "$model" 'tool-search-regex-nonstream' "$key" "$regex_beta" "$regex_payload" false
  sleep 0.5
  capture_server_tool "$model" 'tool-search-regex-stream' "$key" "$regex_beta" "$(printf '%s' "$regex_payload" | jq -c '. + {stream:true}')" true
  sleep 0.5

  bm25_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:256,messages:[{role:"user",content:"Find a deferred tool that can look up the weather for Paris, then invoke it."}],tools:[{type:"tool_search_tool_bm25_20251119",name:"tool_search_tool_bm25"},{name:"get_weather",description:"Get current weather for a city",input_schema:{type:"object",properties:{city:{type:"string"}},required:["city"]},defer_loading:true},{name:"get_time",description:"Get local time for a city",input_schema:{type:"object",properties:{city:{type:"string"}},required:["city"]},defer_loading:true}],tool_choice:{type:"tool",name:"tool_search_tool_bm25"}}')
  capture_server_tool "$model" 'tool-search-bm25-nonstream' "$key" "$regex_beta" "$bm25_payload" false
  sleep 0.5
  capture_server_tool "$model" 'tool-search-bm25-stream' "$key" "$regex_beta" "$(printf '%s' "$bm25_payload" | jq -c '. + {stream:true}')" true
  sleep 0.5

  # Unsupported/optional server tools are captured deliberately: their
  # ValidationException bodies document model-specific availability.
  web_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the web search server tool to search for AWS Bedrock."}],tools:[{type:"web_search_20250305",name:"web_search"}],tool_choice:{type:"tool",name:"web_search"}}')
  capture_server_tool "$model" 'web-search-nonstream' "$key" '' "$web_payload" false
  sleep 0.5

  computer_payload=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the computer server tool to click at coordinates 10,10."}],tools:[{type:"computer_20250124",name:"computer",display_width_px:1024,display_height_px:768,display_number:0}],tool_choice:{type:"tool",name:"computer"}}')
  capture_server_tool "$model" 'computer-nonstream' "$key" 'computer-use-2025-01-24' "$computer_payload" false
  sleep 0.5
done

unset AIT_LOW AIT_HIGH key
printf 'server-tools-complete\n'
