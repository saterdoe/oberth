# Localization contract

Oberth presents product copy in English and Spanish. Source code, paths, model
names, conversation messages, evidence and raw provider diagnostics stay verbatim.
The language setting is a presentation preference, not a translation request to
the model or a transformation of stored data.

## Adding copy

- Use `useI18n().t('area.intent', values)` in React components, including accessible
  names, placeholders, busy states, errors and confirmation dialogs.
- Add the English key and its Spanish counterpart to `ui/src/messages.ts` or
  `ui/src/messages-dynamic.ts`; shared navigation/settings keys are in `i18n.tsx`.
  Spanish catalogs are typed against English keys. Never cast arbitrary product
  strings to `MessageKey` to bypass the compiler.
- Use named parameters instead of concatenating translated sentence fragments.
  Interpolation is single-pass: parameter values are not interpreted as messages.
- For count-dependent messages, use a base plural message and a `.one` variant.
  `Intl.PluralRules` selects the variant; zero uses the base plural message.
- Use `useI18n().format` for costs, numbers, dates and relative times. These
  presentation strings must not be persisted as protocol values.
- Keep technical enum values, source snippets and user-authored text outside
  translation calls. `data-no-translate` is descriptive only; no DOM translator
  runs, so correctness does not depend on remembering that attribute.

## Validation

Run `npm --prefix ui run typecheck` and `npm --prefix ui test`. Tests enforce
catalog key/parameter parity, JSX copy extraction, singular/plural behavior,
Intl formatting and preservation of arbitrary user content on language changes.
The existing browser journey runs in both languages. For manual inspection with
isolated fixtures, build the UI and run `node scripts/browser-e2e.mjs --serve`;
open its printed loopback URL and stop the process when finished. This mode does
not connect to Ollama, a real project, or a personal vault.
