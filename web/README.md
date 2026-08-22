# Wordle web app

The frontend loads the Go solver compiled to WebAssembly. From the repository
root, regenerate the solver and build the static site with:

```sh
make -C wordle wasm
npm install --prefix web
npm run build --prefix web
```

For local development:

```sh
make -C wordle wasm
npm run dev --prefix web
```

The app expects `web/public/wordle.wasm` and `web/public/wasm_exec.js`.
