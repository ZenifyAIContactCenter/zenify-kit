---
name: react-native-patterns
description: Use when writing or changing React Native app code (react-native dependency, ios/ + android/) — design tokens over hex/inline styles, JS-only vs native rebuild, iOS/Android native-module parity, runtime permissions, background push, release logging, device verification.
allowed-tools: Read Grep Glob Bash
---

# React Native patterns

Generic idioms for a React Native app. The repo's own `<repo>-conventions` skill names the concrete
theme module, UI kit and native modules — read it too; where the two differ, the repo skill wins.

## Styling: tokens, not literals

Colors and type come from one theme module (for example `src/theme`), never from literals in a
screen:

```tsx
import { colors, textStyles } from '@theme';

const styles = StyleSheet.create({
  title: { ...textStyles.title, color: colors.text.primary },
  card: { backgroundColor: colors.bg.surface, borderColor: colors.border.default },
});
```

- Name color tokens by **role** (`text.secondary`, `bg.subtle`, `status.danger`), not by hue. A
  raw palette (`gray[500]`) stays inside the theme module; a later dark mode is then a second value
  table, not a rewrite of every screen.
- Take the type scale from a platform standard instead of inventing one: Apple HIG text styles
  (iOS Dynamic Type at the default "Large" size: `largeTitle`, `title1`–`title3`, `headline`, `body`,
  `callout`, `subhead`, `footnote`, `caption1`, `caption2`) or the Material 3 type scale
  (`display`/`headline`/`title`/`body`/`label` × L/M/S). Keep the standard's **role names** and copy
  size, line height and weight from its published table — cite the source in the theme file.
- Do not name type steps `xs`/`sm`/`lg`/`xl`: those read as Tailwind sizes, and a team that also
  ships a Tailwind web app will assume Tailwind values.
- Route text through one text component that takes a style name. Arbitrary one-off sizes are what
  make a scale drift; map an existing app's sizes onto the standard (exact matches first, as a
  no-visual-change step; off-scale sizes to the nearest step as a separate, reviewed change).
- `StyleSheet.create` at module level, not `style={{ … }}` inline: inline objects are recreated on
  every render and cannot be found by a token audit.
- Migrating an existing app: replace only literals that **exactly** match a token (no visual
  change), keep the old constants deprecated rather than changing their values, and treat any value
  change (a new brand color, a normalised font size) as a visible design change that needs
  before/after screenshots.
- Stop regressions mechanically: a lint rule (`no-restricted-syntax` on hex `Literal`s, off inside
  the theme module) plus a counter script whose numbers a PR may only lower.

## Layout

- Pad screens with safe-area insets (`useSafeAreaInsets`) instead of fixed status-bar heights; notch,
  Dynamic Island and Android gesture bars differ per device.
- Keyboard: one shared keyboard-avoiding wrapper per screen type; test with the keyboard open on
  both platforms.

## JS-only vs native change

A change is **native** — needs a rebuild, cannot ship as an over-the-air JS update — when it touches
any of: a native module (`ios/`, `android/`, a podspec, a gradle file), `Info.plist` /
`AndroidManifest.xml` (permissions, background modes, URL schemes), a dependency with native code,
fonts or assets registered natively. Everything else reloads through Metro.

- Before claiming a JS change works, build the bundle (`npx react-native bundle --platform ios
  --dev false --entry-file index.js --bundle-output <tmp>`): it catches broken imports and path
  aliases that the type checker does not (aliases live in both `tsconfig.json` and the Babel
  resolver).
- Before claiming a native change works, build **both** platforms.

## Native modules: keep iOS and Android in step

- One JS-facing API, typed in a wrapper module; both platforms implement every method. A method
  added on one side only is a runtime `undefined is not a function` on the other.
- Events from native carry the same field names on both platforms; read the emitter on each side
  before consuming a field.
- Threading: UI and system call frameworks expect the main thread/queue; long work goes off it. A
  native callback that may arrive on a background thread must hop to main before touching UI or
  system call state.

## Runtime permissions

- Ask at the point of use (before the action that needs it), not all at app start.
- Flow: `check` → if not yet asked, `request` → if blocked/denied, explain and offer to open
  Settings; never loop the system dialog.
- The system dialog only shows while the app is in the foreground; when the trigger happens in the
  background (e.g. answering from the lock screen), defer the prompt to the next `active` state.
- iOS needs the matching usage-description string in `Info.plist`; Android needs the manifest entry
  plus the runtime request.

## Background, push and calls

- Background wake-ups (VoIP / data push) have a tight time budget: do the minimum, defer
  everything else to foreground. Heavy JS boot or log flushing during a background wake can miss the
  deadline and get the app killed.
- The iOS simulator does not deliver VoIP push and has no real call audio — test push and call flows
  on a physical device.
- Assume the JS state is fresh after a background launch: anything cached in memory from a previous
  session is gone.

## Logging

- Release builds commonly strip `console.*` (Babel `transform-remove-console`); production
  diagnostics go through a structured remote logger, one line per decision point, with stable keys
  that can be filtered.
- Never log credentials or full phone numbers; mask personal data before it leaves the device.

## Lists and performance

- `FlatList` / `FlashList` with a stable `keyExtractor`, memoised row components, and no inline
  closures in the row render path.
- For deeper performance work (Hermes profiling, bundle size, re-render tracing) use a dedicated
  React Native performance skill if one is installed; do not guess.

## Verify

- Type check and lint (new lint warnings in touched files are yours to fix).
- JS bundle builds (see above); native build on both platforms when native code changed.
- Run the real flow on a device or simulator; for style changes, screenshot before and after.
