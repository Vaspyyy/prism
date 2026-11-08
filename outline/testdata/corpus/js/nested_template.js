// Templates nested in a ${…} that spans lines, with an apostrophe and a
// "//" inside: none of it ends the outer template early.
const html = `
  ${items.map((item) => `
    <li>Don't forget ${item}: https://example.com/${item}</li>`).join("")}
`;

function after() {}

class Later {
  run() {
    return after();
  }
}

const page = `
  ${(() => {
    const inner = { a: 1 };
    return inner.a;
  })()}
  function notAFunction() {
`;

export function last() {
  return page + html;
}

// Names that start with "function" aren't functions.
const names = {
  functionName,
  functionArn: "arn",
};

function* generate() {}
