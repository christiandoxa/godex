package cli

const pingOpenAIHelpText = `Send a minimal application request through the OpenAI/Codex runtime path.

Usage: godex ping openai [OPTIONS]

Options:
  -p, --profile NAME  Probe only this OpenAI profile
      --model MODEL   Model passed through the normal Codex request path
      --effort LEVEL  Reasoning effort passed through the normal Codex request path
      --base-url URL  Override the ChatGPT backend base URL
      --no-proxy      Bypass upstream proxy settings
      --json          Emit stable JSON output
  -h, --help          Print help
`
