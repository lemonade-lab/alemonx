# ALemonX DSH Runtime

This directory pins the managed DeepSeek Harness dependency shipped with ALemonX.
The user-facing entry starts the official `web` profile on an automatically
assigned loopback port with `--no-open`, then opens its authenticated URL through
the existing embedded browser WebView and HTTP proxy. Chat, settings, sessions,
and approvals belong to the official UI. A small official-loader plugin registers
the selected robot directory and selects its latest non-archived session on
page startup, reusing an empty session when no history exists. It seeds the
official language preference as Simplified Chinese when none is saved, preserving
subsequent user language selections.

Each robot retains its managed `DSH_HOME`; the Web and legacy SDK processes never
write it concurrently. Existing SDK profiles and session data are preserved.
Keyring/Docker Secret credentials are inherited read-only by the Web process;
credentials configured inside the official UI use DSH's own credential store.
The SDK and approval bridge remain for compatibility with existing host APIs.
