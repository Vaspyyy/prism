export function helper(x: number): number {
  return x + 1;
}

export class View {
  render() {
    return helper(1);
  }
}

export const handler = (e: Event) => {
  console.log(e);
};

worker.onmessage = (e) => {
  handler(e);
};
