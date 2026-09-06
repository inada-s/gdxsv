# gdxsv website

React + TypeScript frontend for gdxsv, deployed to GitHub Pages (www.gdxsv.net).
Built with [Vite](https://vite.dev/).

## Development

```
npm install
npm start
```

Starts the Vite dev server at http://localhost:5173.

## Testing

```
npm test        # run tests once (Vitest)
npm run test:watch
```

## Build

```
npm run build
```

Type-checks with `tsc` and builds a production bundle into `build/`.

```
npm run preview
```

Serves the built `build/` output locally for a sanity check.

## Deploy

```
npm run deploy
```

Builds and pushes `build/` to the `gh-pages` branch (publishes to www.gdxsv.net).
Run this intentionally only — it publishes to production.
