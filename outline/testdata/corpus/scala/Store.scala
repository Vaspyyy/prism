package corpus

class Store {
  private val help = """
    class NotAClass {
    object NotAnObject
  """

  def add(item: String): Unit = println(item)
}

object Store {
  def apply(): Store = new Store
}
