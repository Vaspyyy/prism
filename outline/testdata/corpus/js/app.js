// A small app.
import { render } from "./render.js";

/*
function commentedOut() {
  return 1;
}
*/

export function createApp(root) {
  const view = `
    function notAFunction() {
    class NotAClass {}
    ${render(root)}
  `;
  function helper() {
    return view;
  }
  return { helper };
}

const slash = /`/;

const esc = `escaped \` backtick
function stillInTemplate() {
`;

export const total = (items) => {
  return items.length;
};

export class Store {
  constructor() {
    this.items = [];
  }

  add(item) {
    this.items.push(item);
  }
}

worker.onmessage = (e) => {
  console.log(e.data);
};

const handlers = {
  save: (v) => {
    return v;
  },
};

export default function main() {
  return createApp(document.body);
}
