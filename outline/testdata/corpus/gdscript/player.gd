extends Node2D
class_name Player

"""
func not_a_function():
	pass
"""

signal died

enum State { IDLE, RUN }

func _ready():
	"""Set up.

func also_not():
	"""
	pass

func move(delta):
	position.x += delta

class Inner:
	func helper():
		return 1
