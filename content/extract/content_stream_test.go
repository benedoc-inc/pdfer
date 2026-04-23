package extract

import (
	"testing"
)

func TestEffectiveFontSize_ZeroScale(t *testing.T) {
	tests := []struct {
		name     string
		state    textState
		expected float64
	}{
		{
			name: "zero scale text matrix with positive fontSize falls back to fontSize",
			state: textState{
				fontSize:   12,
				textMatrix: [6]float64{0, 0, 0, 0, 0, 0},
				ctm:        [6]float64{1, 0, 0, 1, 0, 0},
			},
			expected: 12,
		},
		{
			name: "normal scale text matrix returns scaled value",
			state: textState{
				fontSize:   12,
				textMatrix: [6]float64{1, 0, 0, 1, 0, 0},
				ctm:        [6]float64{1, 0, 0, 1, 0, 0},
			},
			expected: 12,
		},
		{
			name: "fontSize zero returns zero",
			state: textState{
				fontSize:   0,
				textMatrix: [6]float64{1, 0, 0, 1, 0, 0},
				ctm:        [6]float64{1, 0, 0, 1, 0, 0},
			},
			expected: 0,
		},
		{
			name: "2x scale text matrix doubles fontSize",
			state: textState{
				fontSize:   10,
				textMatrix: [6]float64{2, 0, 0, 2, 0, 0},
				ctm:        [6]float64{1, 0, 0, 1, 0, 0},
			},
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveFontSize(&tt.state)
			// Allow small floating point tolerance
			diff := got - tt.expected
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("effectiveFontSize() = %f, want %f", got, tt.expected)
			}
		})
	}
}
