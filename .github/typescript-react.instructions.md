---
applyTo: '**/*.ts,**/*.tsx'
---

# Project coding standards for TypeScript and React

Apply the [general coding guidelines](./general-coding.instructions.md) to all code.

## TypeScript Guidelines

- Use TypeScript for all new code
- Follow functional programming principles where possible
- Use interfaces for data structures and type definitions
- Prefer immutable data (const, readonly)
- Use optional chaining (?.) and nullish coalescing (??) operators if necessary
- Prefer type narrowing over type assertions
- Use `as const satisfies ...` for key/value constants
- Avoid `any`; prefer `unknown` or precise generics.
- Use utility types (`Partial`, `Pick`, `Omit`, `Record`, `Required`) to transform interfaces.

## React Guidelines

- Use functional components with hooks
- Follow the React hooks rules (no conditional hooks)
- Use React.FC type for components with children
- Keep components small and focused

## Styling

- Use SCSS for component styling (named `ComponentName.module.scss`)
- Use `clsx` for class concatenation and conditional types
