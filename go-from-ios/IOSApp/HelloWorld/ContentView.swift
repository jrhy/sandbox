//
//  ContentView.swift
//  HelloWorld
//

import SwiftUI
import Goo

struct ContentView: View {
    var body: some View {
        Text(Goo.GooGreetings("Googoo"))
            .padding()
    }
}

struct ContentView_Previews: PreviewProvider {
    static var previews: some View {
            Text(googoo())
                .padding()
    }
}

func googoo() -> String {
    return "Hello googoo!"
}

