<script>
  let count = 0;

  function increment() {
    count += 1;
  }

  const label = `
  function notAFunction() {
  `;
</script>

<button on:click={increment}>{label} {count}</button>
