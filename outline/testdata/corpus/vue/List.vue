<template>
  <ul><li v-for="i in items">{{ i }}</li></ul>
</template>

<script>
/*
function commentedOut() {}
*/
export default {
  data() {
    return { items: [] };
  },
};

function sortItems(items) {
  return items.slice().sort();
}
</script>
