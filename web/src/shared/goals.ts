export const GOAL_OPTIONS = [
  'Earn the Boulder Badge.',
  'Earn 2 badges.',
  'Earn 3 badges.',
  'Earn 4 badges.',
  'Earn 5 badges.',
  'Earn 6 badges.',
  'Earn 7 badges.',
  'Earn all 8 badges.',
  'Beat the Elite Four and Champion.',
  'Complete the obtainable Pokédex.',
  ''
] as const

export function nextGoalForPlayStyle(currentGoal: string, playStyle: string, explicitlySelected: boolean): string {
  if (explicitlySelected) return currentGoal
  void playStyle
  return 'Beat the Elite Four and Champion.'
}
