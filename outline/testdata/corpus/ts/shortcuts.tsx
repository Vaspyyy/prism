// A lone backtick in JSX text isn't a template string.
export function Shortcuts() {
  return (
    <ul>
      <li><kbd>`</kbd> opens the console</li>
    </ul>
  );
}

function Other() {
  return null;
}

const Third = () => <p>third</p>;
