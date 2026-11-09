package corpus

enum class Mode(val id: String) {
    FAST("fast"),
    SLOW("slow");

    fun label(): String = id
}

annotation class Marker

@JvmInline
value class Id(val raw: Int)

fun interface Check {
    fun ok(): Boolean
}

abstract class Shape {
    abstract fun area(): Double

    protected open fun name(): String = "shape"

    operator fun plus(other: Shape): Shape = this

    infix fun beside(other: Shape): Boolean = false

    private inner class Corner(val x: Int) {
        fun far(): Boolean = x > 9
    }
}
