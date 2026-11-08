<template>
  <!-- a lone backtick in markup isn't a template string -->
  <p>Press <code>`</code> to open the console.</p>
</template>

<script>
export default {
  methods: {
    save() {
      return true;
    },
  },
};

function helper() {
  return 1;
}
</script>
