package corpus;

/*
public class CommentedOut {
}
*/
public class Store {
    private final String help = """
    public void notAMethod() {
    quoted: \"""
    class NotAClass {
    """;

    public void add(String item) {
        System.out.println(item);
    }

    public static Store create() {
        return new Store();
    }

    interface Listener {
        void changed();
    }

    enum Mode { READ, WRITE }

    record Pair(String a, String b) {}
}
