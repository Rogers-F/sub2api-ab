#!/bin/zsh

set -u

read -s AIT_LOW
printf '\n'
read -s AIT_HIGH
printf '\n'

DOC='/tmp/sub2api-0ait-aws-original-responses.md'
STATUS='/tmp/sub2api-0ait-aws-original-status.tsv'
WORK=$(mktemp -d /tmp/sub2api-0ait-capture.XXXXXX)
printf '%s\n' "$WORK" > /tmp/sub2api-0ait-capture-workdir
: > "$STATUS"

cat > "$DOC" <<'EOF'
# 0ait AWS Bedrock Original Response Capture

Temporary verification document. Authentication values are intentionally excluded.

- Endpoint: `https://api.0ait.com/v1/messages`
- Common request headers: `content-type: application/json`, `anthropic-version: 2023-06-01`
- Scenarios: extended-thinking non-stream, extended-thinking stream, forced-tool non-stream, forced-tool stream, tool-result follow-up
- Response headers omit `set-cookie`.

EOF

complex_prompt='Find every ordered triple of positive integers (x,y,z) with x <= y <= z and 1/x + 1/y + 1/z = 1. Give a proof that the list is complete, then put the final triples on one line. Keep the visible final answer under 180 words.'
tool_prompt='Use the lookup tool to retrieve the value for key alpha. Do not answer from memory.'

append_headers() {
  awk 'tolower($0) ~ /^set-cookie:/ {next} {sub(/\r$/, ""); print}' "$1" >> "$DOC"
}

capture() {
  local model="$1"
  local scenario="$2"
  local key="$3"
  local payload="$4"
  local stream="$5"
  local keep_body="$6"
  local safe_model headers body http_code curl_rc result_type
  local -a stream_arg

  safe_model=$(printf '%s' "$model" | tr -c 'A-Za-z0-9._-' '_')
  headers="$WORK/${safe_model}-${scenario}.headers"
  body="$WORK/${safe_model}-${scenario}.body"
  stream_arg=()
  if [[ "$stream" == true ]]; then
    stream_arg=(-N)
  fi

  http_code=$(curl --http1.1 --retry 3 --retry-all-errors --retry-delay 2 \
    -sS "${stream_arg[@]}" --max-time 180 \
    -D "$headers" -o "$body" -w '%{http_code}' \
    'https://api.0ait.com/v1/messages' \
    -H "Authorization: Bearer $key" \
    -H "x-api-key: $key" \
    -H 'anthropic-version: 2023-06-01' \
    -H 'content-type: application/json' \
    --data-binary "$payload")
  curl_rc=$?

  {
    printf '## %s / %s\n\n' "$model" "$scenario"
    printf '**Request**\n\n````json\n'
    printf '%s' "$payload" | jq . 2>/dev/null || printf '%s\n' "$payload"
    printf '\n````\n\n**HTTP status:** `%s`\n\n' "$http_code"
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

  if [[ -n "$keep_body" ]]; then
    command cp "$body" "$keep_body"
  fi
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

  thinking_payload=$(jq -cn \
    --arg model "$model" \
    --arg prompt "$complex_prompt" \
    '{model:$model,max_tokens:1400,thinking:{type:"enabled",budget_tokens:1024},messages:[{role:"user",content:$prompt}]}')
  capture "$model" 'thinking-nonstream' "$key" "$thinking_payload" false ''
  sleep 1

  thinking_stream_payload=$(printf '%s' "$thinking_payload" | jq -c '. + {stream:true}')
  capture "$model" 'thinking-stream' "$key" "$thinking_stream_payload" true ''
  sleep 1

  tool_payload=$(jq -cn \
    --arg model "$model" \
    --arg prompt "$tool_prompt" \
    '{model:$model,max_tokens:128,messages:[{role:"user",content:$prompt}],tools:[{name:"lookup",description:"Retrieve a value by key",input_schema:{type:"object",properties:{key:{type:"string"}},required:["key"]}}],tool_choice:{type:"tool",name:"lookup"}}')
  safe_model=$(printf '%s' "$model" | tr -c 'A-Za-z0-9._-' '_')
  tool_body="$WORK/${safe_model}-tool-response.json"
  capture "$model" 'tool-nonstream' "$key" "$tool_payload" false "$tool_body"
  sleep 1

  tool_stream_payload=$(printf '%s' "$tool_payload" | jq -c '. + {stream:true}')
  capture "$model" 'tool-stream' "$key" "$tool_stream_payload" true ''
  sleep 1

  if jq -e '.type == "message" and ([.content[]?.type] | index("tool_use") != null)' \
    "$tool_body" >/dev/null 2>&1; then
    assistant_content=$(jq -c '.content' "$tool_body")
    tool_id=$(jq -r '.content[] | select(.type == "tool_use") | .id' "$tool_body" | head -1)
    followup_payload=$(jq -cn \
      --arg model "$model" \
      --argjson assistant "$assistant_content" \
      --arg tool_id "$tool_id" \
      '{model:$model,max_tokens:128,messages:[{role:"user",content:"Use the lookup tool."},{role:"assistant",content:$assistant},{role:"user",content:[{type:"tool_result",tool_use_id:$tool_id,content:"alpha=42"}]}]}')
    capture "$model" 'tool-result-followup' "$key" "$followup_payload" false ''
  else
    printf '%s\t%s\t%s\t%s\t%s\n' \
      "$model" 'tool-result-followup' 'SKIP' '0' 'no_tool_use' >> "$STATUS"
    printf 'skipped %s tool-result-followup: no tool_use response\n' "$model"
  fi
  sleep 1
done

fable_payload=$(jq -cn \
  --arg prompt "$complex_prompt" \
  '{model:"claude-fable-5",max_tokens:1400,thinking:{type:"enabled",budget_tokens:1024},messages:[{role:"user",content:$prompt}]}')
capture 'claude-fable-5' 'thinking-nonstream-unavailable-check' \
  "$AIT_HIGH" "$fable_payload" false ''

{
  printf '\n## Capture Status\n\n````text\n'
  column -t -s $'\t' "$STATUS"
  printf '````\n'
} >> "$DOC"

unset AIT_LOW AIT_HIGH key
printf 'document=%s\n' "$DOC"
printf 'status=%s\n' "$STATUS"
printf 'work=%s\n' "$WORK"
wc -c -l "$DOC" "$STATUS"
