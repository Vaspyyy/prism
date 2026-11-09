package corpus

class Seasons
@Inject
constructor(private val config: Config) : Source {
    override fun getSerializedName(): String = id

    fun length(): Int = try {
        config.length
    } catch (e: Exception) {
        28
    }

    fun rank(n: Int) = if (n > 9) {
        "high"
    } else if (n > 4) {
        "mid"
    } else {
        "low"
    }

    fun able(p: Person): Boolean =
        p.isAlive && !p.isRemoved &&
            p.carried.isEmpty

    fun ghosts(): List<Line> = slots.filter { it.left >= 0f }.map { (id, slot) ->
        Line(id, slot)
    }

    fun <T> first(xs: List<T>): T
        where T : Comparable<T> {
        return xs.first()
    }

    private class Icon(val width: Int, val height: Int)

    fun wet(x: Int, z: Int): Boolean

    @Volatile
    private var seen = 0

    companion object {
        val CODEC = build {
            it.stable()
        }
    }
}
