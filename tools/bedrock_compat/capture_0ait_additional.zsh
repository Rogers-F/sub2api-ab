#!/bin/zsh

set -u

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

capture_extra() {
  local model="$1"
  local scenario="$2"
  local key="$3"
  local payload="$4"
  local stream="$5"
  local safe_model headers body http_code curl_rc result_type
  local -a stream_arg

  safe_model=$(printf '%s' "$model" | tr -c 'A-Za-z0-9._-' '_')
  headers="$WORK/extra-${safe_model}-${scenario}.headers"
  body="$WORK/extra-${safe_model}-${scenario}.body"
  stream_arg=()
  if [[ "$stream" == true ]]; then stream_arg=(-N); fi
  http_code=$(curl --http1.1 --retry 3 --retry-all-errors --retry-delay 2 \
    -sS "${stream_arg[@]}" --max-time 180 \
    -D "$headers" -o "$body" -w '%{http_code}' \
    'https://api.0ait.com/v1/messages' \
    -H "Authorization: Bearer $key" -H "x-api-key: $key" \
    -H 'anthropic-version: 2023-06-01' -H 'content-type: application/json' \
    --data-binary "$payload")
  curl_rc=$?
  {
    printf '## %s / %s\n\n**Request**\n\n````json\n' "$model" "$scenario"
    printf '%s' "$payload" | jq . 2>/dev/null || printf '%s\n' "$payload"
    printf '\n````\n\n**HTTP status:** `%s`\n\n**Response headers**\n\n````http\n' "$http_code"
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
  printf '%s\t%s\t%s\t%s\t%s\n' "$model" "$scenario" "$http_code" "$curl_rc" "$result_type" >> "$STATUS"
  printf 'captured %s %s http=%s rc=%s\n' "$model" "$scenario" "$http_code" "$curl_rc"
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
modern=(claude-opus-4-7 claude-opus-4-8 claude-opus-5 claude-sonnet-5)

for model in "${modern[@]}"; do
  adaptive=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:1400,thinking:{type:"adaptive"},output_config:{effort:"high"},messages:[{role:"user",content:"Find every ordered triple of positive integers (x,y,z) with x <= y <= z and 1/x + 1/y + 1/z = 1. Prove completeness and give the final triples in under 180 words."}]}')
  key="$AIT_HIGH"
  capture_extra "$model" 'adaptive-thinking-nonstream' "$key" "$adaptive" false
  sleep 1
  adaptive_stream=$(printf '%s' "$adaptive" | jq -c '. + {stream:true}')
  capture_extra "$model" 'adaptive-thinking-stream' "$key" "$adaptive_stream" true
  sleep 1
done

for model in "${models[@]}"; do
  case "$model" in
    claude-opus-4-7|claude-opus-4-8|claude-opus-5|claude-sonnet-5) key="$AIT_HIGH" ;;
    *) key="$AIT_LOW" ;;
  esac
  plain=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:8,messages:[{role:"user",content:"Reply with OK only."}]}')
  capture_extra "$model" 'plain-nonstream' "$key" "$plain" false
  sleep 1
  plain_stream=$(printf '%s' "$plain" | jq -c '. + {stream:true}')
  capture_extra "$model" 'plain-stream' "$key" "$plain_stream" true
  sleep 1
done

for model in claude-opus-4-6 claude-haiku-4-5-20251001 claude-opus-4-8 claude-opus-5; do
  case "$model" in claude-opus-4-8|claude-opus-5) key="$AIT_HIGH" ;; *) key="$AIT_LOW" ;; esac
  max_req=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:1,messages:[{role:"user",content:"Write a detailed proof of the classification of all positive integer solutions to 1/x+1/y+1/z=1."}]}')
  capture_extra "$model" 'max-tokens-stop' "$key" "$max_req" false
  sleep 1
  stop_req=$(jq -cn --arg model "$model" \
    '{model:$model,max_tokens:32,stop_sequences:["<END>"],messages:[{role:"user",content:"Reply with exactly <END> and nothing else."}]}')
  capture_extra "$model" 'stop-sequence' "$key" "$stop_req" false
  sleep 1
done

cache_text=$(printf 'This is a stable cache probe sentence with numbered context item %s. ' {1..1100})
for model in claude-opus-4-6 claude-haiku-4-5-20251001 claude-opus-4-8 claude-opus-5; do
  case "$model" in claude-opus-4-8|claude-opus-5) key="$AIT_HIGH" ;; *) key="$AIT_LOW" ;; esac
  cache_req=$(jq -cn --arg model "$model" --arg text "$cache_text" \
    '{model:$model,max_tokens:4,system:[{type:"text",text:$text,cache_control:{type:"ephemeral"}}],messages:[{role:"user",content:"Reply OK."}]}')
  capture_extra "$model" 'cache-creation' "$key" "$cache_req" false
  sleep 2
  capture_extra "$model" 'cache-read' "$key" "$cache_req" false
  sleep 1
done

thinking_tool_prompt='Use the lookup tool to retrieve alpha, and briefly explain the result after the tool returns.'
for model in claude-opus-4-6 claude-opus-4-8; do
  if [[ "$model" == claude-opus-4-8 ]]; then
    key="$AIT_HIGH"
    thinking_spec='{"type":"adaptive"}'
  else
    key="$AIT_LOW"
    thinking_spec='{"type":"enabled","budget_tokens":1024}'
  fi
  thinking_tool=$(jq -cn --arg model "$model" --arg prompt "$thinking_tool_prompt" --argjson thinking "$thinking_spec" \
    '{model:$model,max_tokens:1400,thinking:$thinking,messages:[{role:"user",content:$prompt}],tools:[{name:"lookup",description:"Retrieve a value by key",input_schema:{type:"object",properties:{key:{type:"string"}},required:["key"]}}],tool_choice:{type:"auto"}}')
  if [[ "$model" == claude-opus-4-8 ]]; then
    thinking_tool=$(printf '%s' "$thinking_tool" | jq -c '. + {output_config:{effort:"high"}}')
  fi
  capture_extra "$model" 'thinking-tool-nonstream' "$key" "$thinking_tool" false
  sleep 1
  thinking_tool_stream=$(printf '%s' "$thinking_tool" | jq -c '. + {stream:true}')
  capture_extra "$model" 'thinking-tool-stream' "$key" "$thinking_tool_stream" true
  sleep 1
done

unset AIT_LOW AIT_HIGH key
printf 'additional-complete\n'
