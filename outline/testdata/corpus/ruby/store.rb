class Store
  HELP = <<~TEXT
  def not_a_method
  end
  TEXT

  class << self
    def build
      new
    end
  end

  def add(item)
    items << item
  end

  def self.create
    new
  end
end

module Corpus
  VERSION = "1"
end

=begin
def commented_out
end
=end

def top_level
  Store.create
end
