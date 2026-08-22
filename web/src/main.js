import './style.css';

const board = document.querySelector('#board');
const solveButton = document.querySelector('#solve');
const status = document.querySelector('#status');
const results = document.querySelector('#results');

const rows = Array.from({ length: 6 }, (_, row) => {
  const element = document.createElement('div');
  element.className = 'guess';
  element.innerHTML = `<input aria-label="Guess ${row + 1}" maxlength="5" spellcheck="false" placeholder="GUESS">` +
    `<input aria-label="Yellow positions ${row + 1}" maxlength="5" spellcheck="false" placeholder="..A..">` +
    `<input aria-label="Green positions ${row + 1}" maxlength="5" spellcheck="false" placeholder=".....">`;
  board.append(element);
  return element;
});

let solverReady = false;

async function loadSolver() {
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('/wordle.wasm'), go.importObject);
  go.run(result.instance);
  solverReady = true;
  solveButton.disabled = false;
  status.textContent = 'Ready.';
}

function solve() {
  const guesses = rows.map(row => {
    const [word, yellow, green] = row.querySelectorAll('input');
    return { word: word.value, yellow: yellow.value, green: green.value };
  }).filter(guess => guess.word.trim());
  const response = JSON.parse(wordleSolve(JSON.stringify({ guesses })));
  results.replaceChildren();
  if (response.error) {
    status.textContent = response.error;
    return;
  }
  status.textContent = `${response.candidates.length} candidates`;
  for (const candidate of response.candidates) {
    const item = document.createElement('li');
    item.textContent = `${candidate.word} (rank ${candidate.freq})`;
    results.append(item);
  }
}

solveButton.addEventListener('click', solve);
loadSolver().catch(error => { status.textContent = `Could not load solver: ${error}`; });
