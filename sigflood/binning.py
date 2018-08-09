"""
Naive implementation to find most likely attacker just based on reference number

Analysis:
n^2 ==> if not in, add - O(n) to loop through paths, O(n) to loop through nodes in path, O(1) to check if in and insert.

Improvements:
Require an attacker's references to be a certain percentage of total paths to be returned as an attacker
Could check only front of paths first because attacker can only spoof the old stuff - could make it faster. (So breadth first instead of depth). Slower with the cash potentially because jumping around lists.
"""

def FindAttacker(paths, num_attackers=1):
	# Form dictionary of node:references
	all_nodes = {}
	for path in paths:
		for node in path:
			if node not in all_nodes:
				all_nodes[node] = 0
			all_nodes[node] += 1

	# Use dictionary to get the top num_attackers references
	return [v[0] for v in sorted(all_nodes.items(), reverse=True, key=lambda tup: tup[1])[:num_attackers]]

def GenerateScenario():
	# Specify total number of total real users
	users = 10

	# Total # of attackers
	attackers = 1

	# Attacker's bandwidth scale compared to user
	scale = 100

	# Randomly select interval(s) in range of users to use as attacker(s)
	nodes = [str(i) for i in range(0, users+attackers)]
	
	eve = [nodes[4]]

	# Construct n paths of length k (or can alternate length)
	# with probability of attacker's interval scale times the others

	paths = [["1", "2", "3"], ["4", "5", "6"], ["7", "5", "6"], ["8", "5", "6"], ["4", "5", "9"], ["7", "5", "9"], ["8", "5", "9"], ["10", "11", "12"]]

	return paths, eve
	
paths, eve = GenerateScenario()

print("Predicted Attacker ASes:", ", ".join(FindAttacker(paths, num_attackers=1)))
print("Real Attacker ASes:", ", ".join(eve))
