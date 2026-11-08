package corpus

data class Item(val name: String)

class Store {
    private val help = """
    fun notAFunction() {
    class NotAClass
    """

    fun add(item: Item) {
        println(item)
    }

    suspend fun load(): List<Item> = emptyList()
}

/*
fun commentedOut() {}
*/
fun <T> List<T>.second(): T = this[1]

object Registry {
    fun all(): List<Store> = emptyList()
}
