namespace Corpus;

public class Store
{
    private const string Help = @"say ""hi""
    public void NotAMethod() {
";

    private const string Raw = """
    public class NotAClass {
    """;

    public void Add(string item)
    {
        System.Console.WriteLine(item);
    }

    public static Store Create()
    {
        return new Store();
    }
}

/*
public class CommentedOut {}
*/
public interface IListener
{
    void Changed();
}
