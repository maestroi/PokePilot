from pathlib import Path

p = Path("agent/llm_plan_request_test.go")
s = p.read_text()
old = 'body := `\\{"model\\":\\"test-model\\",\\"choices\\":[\\{"message\\":\\{"content\\":\\"{\\\\\\"goal\\\\\\":\\\\\\"go north\\\\\\",\\\\\\"steps\\\\\\":[\\\\\\"go to route 1\\\\\\"]}\\"},\\"finish_reason\\":\\"stop\\"}]} `'
# Match the exact generated line more robustly by replacing its whole trimmed line.
lines = s.splitlines()
for i, line in enumerate(lines):
    if line.strip().startswith('body := `'):
        lines[i] = '        body := `{"model":"test-model","choices":[{"message":{"content":"{\\"goal\\":\\"go north\\",\\"steps\\":[\\"go to route 1\\"]}"},"finish_reason":"stop"}]}`'
        break
else:
    raise SystemExit("generated strategist response body line not found")
p.write_text("\n".join(lines) + "\n")
