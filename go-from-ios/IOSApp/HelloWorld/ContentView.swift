//
//  ContentView.swift
//  HelloWorld
//

import SwiftUI
import Goo

struct ContentView: View {
    
    @State var toggleMe = false
    @State var toggles = 0
    
    var body: some View {

        VStack {
            Toggle(isOn:$toggleMe) {
                Text("Toggle Me")
            }
            
            Text("you toggled it " + String(toggles))
                .onChange(of: toggleMe) {value in
                    toggles = Goo.GooInc(toggles)
                }
            
            Spacer()
        }
        
    }
}

struct ContentView_Previews: PreviewProvider {
    static var previews: some View {
        ContentView()
    }
}
